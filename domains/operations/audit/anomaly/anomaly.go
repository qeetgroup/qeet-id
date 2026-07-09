// Package anomaly scores the existing hash-chained audit.events log against
// a per-(tenant, actor) behavioral baseline, flagging deviations — a
// first-time action type, an unusual hour, a brand-new IP — for admin
// review. It is the first behavioral-baseline detector in the codebase;
// domains/access/threat-detection/* score authentication-time signals
// (failed logins, UA heuristics) against static per-tenant thresholds, not a
// rolling history, and write to a separate table family (auth.security_events
// / auth.bot_events) — this package is a distinct, admin-action-focused
// concern layered directly on audit.events.
//
// A background Sweep scores newly-recorded events in small batches (marking
// each audit.events row's scored_at so it's processed exactly once, mirroring
// platform.outbox's published_at IS NULL convention) rather than hooking into
// audit.Record synchronously — that would couple every domain that writes
// audit events to this package and add latency to writes that don't need it.
//
// Deliberately out of scope: events with no human actor (agents/service
// principals already have their own governance surface — see
// domains/federation/adminportal and domains/developer/agents) are folded
// into no baseline and never scored.
package anomaly

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qeetgroup/qeet-id/platform/api/rest/errs"
	"github.com/qeetgroup/qeet-id/platform/api/rest/httpx"
)

const (
	sweepInterval = time.Minute
	sweepBatch    = 200

	defaultScoreThreshold    = 0.6
	defaultMinBaselineEvents = 20

	// Scoring weights. Action novelty is the strongest signal (a first-time
	// action type from this actor is the clearest "this doesn't look like
	// them" signal); IP is the weakest, since legitimate admins travel and
	// switch networks far more often than they change what they do.
	weightAction = 0.5
	weightHour   = 0.3
	weightIP     = 0.2
)

// Anomaly is a flagged deviation, enriched with the underlying event's
// details for display.
type Anomaly struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	EventID      uuid.UUID  `json:"event_id"`
	ActorUserID  *uuid.UUID `json:"actor_user_id"`
	ActorEmail   *string    `json:"actor_email"`
	Score        float64    `json:"score"`
	Reasons      []string   `json:"reasons"`
	Status       string     `json:"status"`
	ResolvedAt   *time.Time `json:"resolved_at"`
	ResolvedBy   *uuid.UUID `json:"resolved_by"`
	CreatedAt    time.Time  `json:"created_at"`
	Action       string     `json:"action"`
	ResourceType string     `json:"resource_type"`
	IP           *string    `json:"ip"`
	EventAt      time.Time  `json:"event_at"`
}

type Settings struct {
	TenantID          uuid.UUID `json:"tenant_id"`
	Enabled           bool      `json:"enabled"`
	ScoreThreshold    float64   `json:"score_threshold"`
	MinBaselineEvents int       `json:"min_baseline_events"`
}

type Summary struct {
	Open       int `json:"open"`
	Resolved7d int `json:"resolved_7d"`
	HighScore  int `json:"high_score_open"` // open anomalies scoring >= 0.85
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// baseline is the counter state for one (tenant, actor) pair.
type baseline struct {
	eventCount int64
	actions    map[string]int64
	hours      map[string]int64
	ips        map[string]int64
}

func emptyBaseline() baseline {
	return baseline{actions: map[string]int64{}, hours: map[string]int64{}, ips: map[string]int64{}}
}

func (s *Service) loadBaseline(ctx context.Context, tx pgx.Tx, tenantID, actorID uuid.UUID) (baseline, error) {
	b := emptyBaseline()
	var actionsJSON, hoursJSON, ipsJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT event_count, actions, hours, ips
		FROM audit.actor_baselines WHERE tenant_id = $1 AND actor_user_id = $2
		FOR UPDATE
	`, tenantID, actorID).Scan(&b.eventCount, &actionsJSON, &hoursJSON, &ipsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	_ = json.Unmarshal(actionsJSON, &b.actions)
	_ = json.Unmarshal(hoursJSON, &b.hours)
	_ = json.Unmarshal(ipsJSON, &b.ips)
	return b, nil
}

func (s *Service) saveBaseline(ctx context.Context, tx pgx.Tx, tenantID, actorID uuid.UUID, b baseline) error {
	actionsJSON, _ := json.Marshal(b.actions)
	hoursJSON, _ := json.Marshal(b.hours)
	ipsJSON, _ := json.Marshal(b.ips)
	_, err := tx.Exec(ctx, `
		INSERT INTO audit.actor_baselines (tenant_id, actor_user_id, event_count, actions, hours, ips, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (tenant_id, actor_user_id) DO UPDATE SET
			event_count = EXCLUDED.event_count,
			actions     = EXCLUDED.actions,
			hours       = EXCLUDED.hours,
			ips         = EXCLUDED.ips,
			updated_at  = NOW()
	`, tenantID, actorID, b.eventCount, actionsJSON, hoursJSON, ipsJSON)
	return err
}

// score compares one event against the actor's baseline (as it stood before
// this event) and returns a 0..1 anomaly score plus the reasons contributing
// to it. Each signal is a novelty/rarity measure, not a statistical model —
// explainable by design, matching the rest of the platform's "explain"
// philosophy (RBAC/ReBAC ?explain=true, AuthZEN context.explain).
func score(b baseline, action, ip string, hour int) (float64, []string) {
	var total float64
	var reasons []string

	if b.actions[action] == 0 {
		total += weightAction
		reasons = append(reasons, "new_action_type")
	}

	hourKey := hourBucket(hour)
	hourFreq := 0.0
	if b.eventCount > 0 {
		hourFreq = float64(b.hours[hourKey]) / float64(b.eventCount)
	}
	// A never-seen hour scores the full weight; a common hour scores near
	// zero. 8 is a smoothing factor so "seen a handful of times" still reads
	// as somewhat unusual rather than snapping straight to zero.
	hourNovelty := 1 - hourFreq*8
	if hourNovelty < 0 {
		hourNovelty = 0
	}
	if hourNovelty > 0 {
		total += weightHour * hourNovelty
	}
	if hourNovelty >= 0.5 {
		reasons = append(reasons, "unusual_hour")
	}

	if ip != "" && b.ips[ip] == 0 {
		total += weightIP
		reasons = append(reasons, "new_ip")
	}

	if total > 1 {
		total = 1
	}
	return total, reasons
}

func hourBucket(hour int) string { return strconv.Itoa(hour) }

func fold(b baseline, action, ip string, hour int) baseline {
	b.eventCount++
	b.actions[action]++
	b.hours[hourBucket(hour)]++
	if ip != "" {
		b.ips[ip]++
	}
	return b
}

// unscoredEvent is the minimal projection of audit.events the scorer needs.
type unscoredEvent struct {
	ID          uuid.UUID
	TenantID    *uuid.UUID
	ActorUserID *uuid.UUID
	Action      string
	IP          *string
	CreatedAt   time.Time
}

func (s *Service) settingsFor(ctx context.Context, tenantID uuid.UUID) (Settings, error) {
	st := Settings{TenantID: tenantID, Enabled: true, ScoreThreshold: defaultScoreThreshold, MinBaselineEvents: defaultMinBaselineEvents}
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, score_threshold, min_baseline_events
		FROM audit.anomaly_settings WHERE tenant_id = $1
	`, tenantID).Scan(&st.Enabled, &st.ScoreThreshold, &st.MinBaselineEvents)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	return st, err
}

// tick processes one batch of unscored events. Each event is handled in its
// own transaction so a single bad row can't block the rest of the batch, and
// so the per-tenant advisory-lock-free baseline read/update stays small.
func (s *Service) tick(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor_user_id, action, host(ip), created_at
		FROM audit.events
		WHERE scored_at IS NULL
		ORDER BY created_at, id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, sweepBatch)
	if err != nil {
		return err
	}
	var batch []unscoredEvent
	for rows.Next() {
		var e unscoredEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorUserID, &e.Action, &e.IP, &e.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, e)
	}
	rows.Close()

	for _, e := range batch {
		if err := s.scoreOne(ctx, e); err != nil {
			slog.Warn("audit anomaly scoring failed", "event", e.ID, "err", err)
		}
	}
	return nil
}

func (s *Service) scoreOne(ctx context.Context, e unscoredEvent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// No human actor (agent/service/system) — nothing to baseline. Mark
	// scored and move on.
	if e.TenantID == nil || e.ActorUserID == nil {
		if _, err := tx.Exec(ctx, `UPDATE audit.events SET scored_at = NOW() WHERE id = $1`, e.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	settings, err := s.settingsFor(ctx, *e.TenantID)
	if err != nil {
		return err
	}

	ip := ""
	if e.IP != nil {
		if addr, perr := netip.ParseAddr(*e.IP); perr == nil {
			ip = addr.String()
		}
	}
	hour := e.CreatedAt.UTC().Hour()

	if settings.Enabled {
		b, err := s.loadBaseline(ctx, tx, *e.TenantID, *e.ActorUserID)
		if err != nil {
			return err
		}
		if b.eventCount >= int64(settings.MinBaselineEvents) {
			sc, reasons := score(b, e.Action, ip, hour)
			if sc >= settings.ScoreThreshold {
				if _, err := tx.Exec(ctx, `
					INSERT INTO audit.anomalies (tenant_id, event_id, actor_user_id, score, reasons)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (event_id) DO NOTHING
				`, *e.TenantID, e.ID, *e.ActorUserID, sc, reasons); err != nil {
					return err
				}
			}
		}
		b = fold(b, e.Action, ip, hour)
		if err := s.saveBaseline(ctx, tx, *e.TenantID, *e.ActorUserID, b); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE audit.events SET scored_at = NOW() WHERE id = $1`, e.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Sweep runs one scoring batch — exported for tests and the scheduler.
func (s *Service) Sweep(ctx context.Context) error { return s.tick(ctx) }

// Run is the background sweeper, registered as a worker (mirrors
// retention.Service.Run / gdpr.Service.Run).
func (s *Service) Run(ctx context.Context) {
	tk := time.NewTicker(sweepInterval)
	defer tk.Stop()
	for {
		select {
		case <-tk.C:
			if err := s.tick(ctx); err != nil {
				slog.Warn("audit anomaly sweep", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

const listCols = `
	a.id, a.tenant_id, a.event_id, a.actor_user_id, u.email,
	a.score, a.reasons, a.status, a.resolved_at, a.resolved_by, a.created_at,
	e.action, e.resource_type, host(e.ip), e.created_at
`

func scanAnomaly(row pgx.Row) (*Anomaly, error) {
	var a Anomaly
	if err := row.Scan(&a.ID, &a.TenantID, &a.EventID, &a.ActorUserID, &a.ActorEmail,
		&a.Score, &a.Reasons, &a.Status, &a.ResolvedAt, &a.ResolvedBy, &a.CreatedAt,
		&a.Action, &a.ResourceType, &a.IP, &a.EventAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// List returns a tenant's anomalies, most recent first. status filters to
// "open"/"resolved"; empty returns both.
func (s *Service) List(ctx context.Context, tenantID uuid.UUID, status string, limit int) ([]Anomaly, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `
		SELECT ` + listCols + `
		FROM audit.anomalies a
		JOIN audit.events e ON e.id = a.event_id
		LEFT JOIN "user".users u ON u.id = a.actor_user_id
		WHERE a.tenant_id = $1
	`
	args := []any{tenantID}
	if status != "" {
		args = append(args, status)
		q += ` AND a.status = $` + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	q += ` ORDER BY a.created_at DESC LIMIT $` + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Anomaly{}
	for rows.Next() {
		a, err := scanAnomaly(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Service) Summary(ctx context.Context, tenantID uuid.UUID) (*Summary, error) {
	var sum Summary
	err := s.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'resolved' AND resolved_at > NOW() - INTERVAL '7 days'),
			count(*) FILTER (WHERE status = 'open' AND score >= 0.85)
		FROM audit.anomalies WHERE tenant_id = $1
	`, tenantID).Scan(&sum.Open, &sum.Resolved7d, &sum.HighScore)
	return &sum, err
}

func (s *Service) Resolve(ctx context.Context, tenantID, id, resolvedBy uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE audit.anomalies SET status = 'resolved', resolved_at = NOW(), resolved_by = $3
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
	`, id, tenantID, resolvedBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (s *Service) GetSettings(ctx context.Context, tenantID uuid.UUID) (*Settings, error) {
	st, err := s.settingsFor(ctx, tenantID)
	return &st, err
}

func (s *Service) UpdateSettings(ctx context.Context, tenantID uuid.UUID, in Settings) (*Settings, error) {
	if in.ScoreThreshold < 0 || in.ScoreThreshold > 1 {
		return nil, errs.ErrUnprocessable.WithDetail("score_threshold must be between 0 and 1")
	}
	if in.MinBaselineEvents < 0 {
		return nil, errs.ErrUnprocessable.WithDetail("min_baseline_events must be >= 0")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit.anomaly_settings (tenant_id, enabled, score_threshold, min_baseline_events, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id) DO UPDATE SET
			enabled             = EXCLUDED.enabled,
			score_threshold     = EXCLUDED.score_threshold,
			min_baseline_events = EXCLUDED.min_baseline_events,
			updated_at          = NOW()
	`, tenantID, in.Enabled, in.ScoreThreshold, in.MinBaselineEvents)
	if err != nil {
		return nil, err
	}
	out := in
	out.TenantID = tenantID
	return &out, nil
}

// =====================================================================
// Handler
// =====================================================================

type Handler struct {
	Service *Service
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/tenants/{tenantID}/audit/anomalies", h.list)
	r.Get("/tenants/{tenantID}/audit/anomalies/summary", h.summary)
	r.Post("/tenants/{tenantID}/audit/anomalies/{id}/resolve", h.resolve)
	r.Get("/tenants/{tenantID}/audit/anomaly-settings", h.getSettings)
	r.Put("/tenants/{tenantID}/audit/anomaly-settings", h.updateSettings)
}

func requirePathTenant(r *http.Request) (uuid.UUID, error) {
	pathTenant, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		return uuid.Nil, errs.ErrBadRequest.WithDetail("invalid tenantID")
	}
	scope, err := httpx.RequireTenant(r)
	if err != nil {
		return uuid.Nil, err
	}
	if pathTenant != scope {
		return uuid.Nil, errs.ErrForbidden.WithDetail("tenant mismatch")
	}
	return scope, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requirePathTenant(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && status != "open" && status != "resolved" {
		httpx.WriteError(w, r, errs.ErrBadRequest.WithDetail("status must be \"open\" or \"resolved\""))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := h.Service.List(r.Context(), tenantID, status, limit)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requirePathTenant(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.Service.Summary(r.Context(), tenantID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requirePathTenant(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, errs.ErrBadRequest.WithDetail("invalid id"))
		return
	}
	p := httpx.PrincipalFromCtx(r.Context())
	if p == nil || p.UserID == nil {
		httpx.WriteError(w, r, errs.ErrUnauthorized.WithDetail("resolve must be attributed to a human"))
		return
	}
	if err := h.Service.Resolve(r.Context(), tenantID, id, *p.UserID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requirePathTenant(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.Service.GetSettings(r.Context(), tenantID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requirePathTenant(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in Settings
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.Service.UpdateSettings(r.Context(), tenantID, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

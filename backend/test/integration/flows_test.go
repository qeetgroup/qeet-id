//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/qeetgroup/qeet-identity/internal/analytics"
	"github.com/qeetgroup/qeet-identity/internal/audit"
	"github.com/qeetgroup/qeet-identity/internal/auth"
	"github.com/qeetgroup/qeet-identity/internal/group"
	"github.com/qeetgroup/qeet-identity/internal/platform/errs"
	"github.com/qeetgroup/qeet-identity/internal/platform/tokens"
	"github.com/qeetgroup/qeet-identity/internal/tenant"
	"github.com/qeetgroup/qeet-identity/internal/user"
	"github.com/qeetgroup/qeet-identity/internal/webhook"
)

func newAuth() (*auth.Service, *user.Repository) {
	users := user.NewRepository(testPool)
	issuer := tokens.NewIssuer("integration-test-signing-secret-key", "qeet", "qeet", 15*time.Minute, 720*time.Hour)
	return auth.NewService(testPool, users, issuer), users
}

// Signup is tenant-less, login works, refresh rotates, and reusing a rotated
// refresh token is treated as theft (revokes the session).
func TestAuthSignupLoginRefreshReuse(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	svc, _ := newAuth()
	email := uniqueSlug("user") + "@example.com"

	pair, u, brief, err := svc.Signup(ctx, auth.SignupInput{Email: email, Password: "password123"})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if u.TenantID != uuid.Nil || brief != nil || pair.TenantID != nil {
		t.Fatalf("signup should be tenant-less: tenantID=%v brief=%v pair.TenantID=%v", u.TenantID, brief, pair.TenantID)
	}

	lp, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	rotated, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: lp.RefreshToken})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rotated.RefreshToken == lp.RefreshToken {
		t.Fatal("refresh should rotate the token")
	}

	// Reusing the now-consumed token must fail (theft detection).
	if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: lp.RefreshToken}); err == nil {
		t.Fatal("reusing a consumed refresh token should fail")
	}
	// ...and that revokes the session, so the freshly-rotated token is dead too.
	if _, err := svc.Refresh(ctx, auth.RefreshInput{RefreshToken: rotated.RefreshToken}); err == nil {
		t.Fatal("session should be revoked after reuse, rotated token must fail")
	}
}

// CreateWithOwner creates the tenant, an owner role granted all permissions, a
// membership row, and adopts the tenant as the creator's home.
func TestTenantCreateWithOwner(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	svc, users := newAuth()

	_, u, _, err := svc.Signup(ctx, auth.SignupInput{Email: uniqueSlug("owner") + "@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}

	repo := tenant.NewRepository(testPool)
	tn, err := repo.CreateWithOwner(ctx, tenant.CreateInput{Slug: uniqueSlug("acme"), Name: "Acme"}, u.ID)
	if err != nil {
		t.Fatalf("CreateWithOwner: %v", err)
	}

	var roleName string
	var isSystem bool
	if err := testPool.QueryRow(ctx, `
		SELECT r.name, r.is_system
		FROM rbac.user_roles ur JOIN rbac.roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND ur.tenant_id = $2
	`, u.ID, tn.ID).Scan(&roleName, &isSystem); err != nil {
		t.Fatalf("owner membership not found: %v", err)
	}
	if roleName != "owner" || !isSystem {
		t.Fatalf("expected system owner role, got %q system=%v", roleName, isSystem)
	}

	// Home tenant adopted (was tenant-less at signup).
	got, err := users.Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.TenantID != tn.ID {
		t.Fatalf("home tenant = %v, want %v", got.TenantID, tn.ID)
	}
}

// Phase-1 regression: webhook subscriptions are only reachable within their
// own tenant — a foreign tenant id yields NotFound, not the row.
func TestWebhookTenantIsolation(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	tenantA := createTenant(t, ctx, uniqueSlug("a"))
	tenantB := createTenant(t, ctx, uniqueSlug("b"))

	svc := webhook.NewService(testPool)
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	sub, err := svc.Create(ctx, tx, webhook.CreateInput{TenantID: tenantA, URL: "https://example.com/hook", Events: []string{}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, err := svc.Get(ctx, sub.ID, tenantA); err != nil {
		t.Fatalf("owner tenant should read its subscription: %v", err)
	}
	if _, err := svc.Get(ctx, sub.ID, tenantB); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("foreign tenant Get should be NotFound, got %v", err)
	}
}

// The refactored group service owns the tx and writes the audit row in it; the
// audit hash-chain must get a row, and Delete is tenant-scoped + idempotent-404.
func TestGroupServiceAuditedFlow(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	tenantID := createTenant(t, ctx, uniqueSlug("grp"))
	svc := group.NewService(testPool)
	actor := audit.Actor{Type: "system"}

	g, err := svc.Create(ctx, group.CreateInput{TenantID: tenantID, Name: "Engineering"}, actor)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var audits int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM audit.events
		WHERE tenant_id = $1 AND action = 'group.created' AND resource_id = $2
	`, tenantID, g.ID).Scan(&audits); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if audits != 1 {
		t.Fatalf("expected 1 group.created audit row, got %d", audits)
	}

	if got, err := svc.List(ctx, tenantID); err != nil || len(got) != 1 {
		t.Fatalf("list = %v (err %v), want 1", got, err)
	}
	if err := svc.Delete(ctx, g.ID, tenantID, actor); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(ctx, g.ID, tenantID, actor); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("second delete should be NotFound, got %v", err)
	}
}

// Every analytics projection must run against the real schema (this catches
// queries that reference missing/out-of-scope columns, like the weekly-
// activity bug). An empty tenant is fine — we only assert it doesn't error.
func TestAnalyticsOverviewRuns(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	tenantID := createTenant(t, ctx, uniqueSlug("an"))
	if _, err := analytics.NewReader(testPool).Overview(ctx, tenantID); err != nil {
		t.Fatalf("analytics overview: %v", err)
	}
}

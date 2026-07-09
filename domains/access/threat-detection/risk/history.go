package risk

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// deviceKey normalizes a User-Agent string to a coarse browser+OS pair (e.g.
// "chrome-macos"). It's a proxy for "device identity" — this codebase has no
// fingerprinting library on the frontend and adding one is a separate,
// larger, privacy-sensitive undertaking — but browser+OS is already more
// specific than the raw bot.Score(ua) heuristic and is enough to tell "a
// completely new device" from "the same one as always."
func deviceKey(ua string) string {
	u := strings.ToLower(ua)

	browser := "other"
	switch {
	case strings.Contains(u, "edg/"):
		browser = "edge"
	case strings.Contains(u, "chrome/"):
		browser = "chrome"
	case strings.Contains(u, "firefox/"):
		browser = "firefox"
	case strings.Contains(u, "safari/"):
		browser = "safari"
	}

	// iOS UAs contain "like Mac OS X" (e.g. "CPU iPhone OS 17_0 like Mac OS
	// X"), so the iphone/ipad check must come before the "mac os" substring
	// match or every iOS device misclassifies as macOS.
	os := "other"
	switch {
	case strings.Contains(u, "windows"):
		os = "windows"
	case strings.Contains(u, "iphone"), strings.Contains(u, "ipad"):
		os = "ios"
	case strings.Contains(u, "mac os"):
		os = "macos"
	case strings.Contains(u, "android"):
		os = "android"
	case strings.Contains(u, "linux"):
		os = "linux"
	}

	return browser + "-" + os
}

// lastCountry returns the most recent country recorded for this user (across
// any device), and whether one was found at all. Rows with no country ("")
// are skipped — they carry no geo signal to compare against.
func (s *Service) lastCountry(ctx context.Context, tenantID, userID uuid.UUID) (country string, seenAt time.Time, ok bool) {
	err := s.pool.QueryRow(ctx, `
		SELECT country, seen_at FROM auth.login_context_history
		WHERE tenant_id = $1 AND user_id = $2 AND country IS NOT NULL AND country <> ''
		ORDER BY seen_at DESC LIMIT 1
	`, tenantID, userID).Scan(&country, &seenAt)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("risk: lookup last country", "err", err)
		}
		return "", time.Time{}, false
	}
	return country, seenAt, true
}

// deviceSeenBefore reports whether this exact device key has ever been
// recorded for this user, at any point in the (unbounded) history — device
// reputation, once earned, doesn't expire the way a trusted-device cookie
// does.
func (s *Service) deviceSeenBefore(ctx context.Context, tenantID, userID uuid.UUID, dk string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM auth.login_context_history
			WHERE tenant_id = $1 AND user_id = $2 AND device_key = $3
		)
	`, tenantID, userID, dk).Scan(&exists)
	return exists, err
}

// recordLogin appends this login's device/country to the user's history.
// Best-effort: a failure here shouldn't fail the login it's describing, so
// errors are logged, not returned.
func (s *Service) recordLogin(ctx context.Context, tenantID, userID uuid.UUID, dk, country string) {
	var c any
	if country != "" {
		c = country
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO auth.login_context_history (tenant_id, user_id, device_key, country)
		VALUES ($1, $2, $3, $4)
	`, tenantID, userID, dk, c); err != nil {
		slog.Warn("risk: record login context", "err", err)
	}
}

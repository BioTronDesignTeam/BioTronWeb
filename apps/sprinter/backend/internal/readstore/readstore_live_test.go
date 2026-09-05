package readstore

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// These run against the real read-only role, because the grants are the whole
// point of this package and no compiler checks them. Set
// SPRINTER_TEST_READ_DATABASE_URL; without it the tests skip.
//
//	SPRINTER_TEST_READ_DATABASE_URL='postgresql://sprinter_reader:change-me-reader@127.0.0.1:15433/biotron' \
//	  go test ./internal/readstore/ -run Live -v
//
// Nothing here writes. The role could not write if it tried, which is what the
// last test proves.
func liveStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("SPRINTER_TEST_READ_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SPRINTER_TEST_READ_DATABASE_URL is unset")
	}
	ctx := context.Background()
	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)
	return store, ctx
}

func TestLiveLogQueriesRunAndPage(t *testing.T) {
	store, ctx := liveStore(t)

	rows, err := store.RecentLogs(ctx, LogQuery{Limit: 5})
	if err != nil {
		t.Fatalf("recent logs: %v", err)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].CreatedAt.After(rows[i-1].CreatedAt) {
			t.Fatal("rows must come back newest first")
		}
	}

	// Every filter is exercised together, because each one is a fragment
	// concatenated into the statement and a wrong placeholder number only
	// shows up when the fragments are combined.
	from := time.Now().Add(-90 * 24 * time.Hour)
	to := time.Now()
	filtered, err := store.RecentLogs(ctx, LogQuery{
		Service: "logger-api", Level: "info", Search: "e",
		From: &from, To: &to, Limit: 3,
	})
	if err != nil {
		t.Fatalf("filtered logs: %v", err)
	}
	for _, row := range filtered {
		if row.Service != "logger-api" || row.Level != "info" {
			t.Fatalf("filter leaked: %+v", row)
		}
	}

	page, next, err := store.LogHistory(ctx, LogQuery{Limit: 1})
	if err != nil {
		t.Fatalf("log history: %v", err)
	}
	if len(page) == 0 {
		t.Skip("the logger schema has no rows to page through")
	}
	if next == "" {
		t.Skip("only one log row exists, so there is no second page")
	}
	second, _, err := store.LogHistory(ctx, LogQuery{Limit: 1, Cursor: next})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) == 1 && second[0].ID == page[0].ID {
		t.Fatal("the cursor returned the row it was meant to move past")
	}
}

func TestLiveLogHistoryRefusesABadCursor(t *testing.T) {
	store, ctx := liveStore(t)
	if _, _, err := store.LogHistory(ctx, LogQuery{Limit: 1, Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("a cursor this package did not write must be refused")
	}
}

func TestLiveHealthHistoryRuns(t *testing.T) {
	store, ctx := liveStore(t)
	rows, err := store.HealthHistory(ctx, "logger-api", time.Now().Add(-24*time.Hour), 200)
	if err != nil {
		t.Fatalf("health history: %v", err)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].CheckedAt.After(rows[i-1].CheckedAt) {
			t.Fatal("checks must come back newest first")
		}
	}
}

func TestLiveAccessJoinsRun(t *testing.T) {
	store, ctx := liveStore(t)
	report, err := store.Access(ctx, "sprinter")
	if err != nil {
		t.Fatalf("access: %v", err)
	}
	if report.AppID != "sprinter" {
		t.Fatalf("report = %+v", report)
	}
	for _, grant := range report.Grants {
		if grant.Login == "" {
			t.Fatalf("a grant came back without an operator: %+v", grant)
		}
	}
	for _, manager := range report.Managers {
		if !manager.IsManager {
			t.Fatalf("a non-manager is listed as one: %+v", manager)
		}
	}
	for _, superuser := range report.Superusers {
		if !superuser.IsSuperuser {
			t.Fatalf("a non-superuser is listed as one: %+v", superuser)
		}
	}

	// An app nobody registered is an answer, not a failure.
	missing, err := store.Access(ctx, "no-such-app")
	if err != nil {
		t.Fatalf("access to an unknown app: %v", err)
	}
	if missing.AppFound {
		t.Fatal("an unregistered app must not report as found")
	}
}

// TestLiveReadRoleCannotSeeGuestKeys is the containment test. Everything else
// in this package assumes the role is narrow; this proves it. A guest key is a
// live credential, so a bot that could read one could hand one out.
func TestLiveReadRoleCannotSeeGuestKeys(t *testing.T) {
	store, ctx := liveStore(t)
	var count int64
	err := store.pool.QueryRow(ctx, `SELECT count(*) FROM oauth.guest_keys`).Scan(&count)
	if err == nil {
		t.Fatal("the read role can select from oauth.guest_keys; it must not")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("want a permission error, got %v", err)
	}

	// Sessions are the same kind of secret, and the same grant must be absent.
	err = store.pool.QueryRow(ctx, `SELECT count(*) FROM oauth.sessions`).Scan(&count)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("the read role must not select from oauth.sessions, got %v", err)
	}
}

func TestLiveReadRoleCannotWrite(t *testing.T) {
	store, ctx := liveStore(t)
	_, err := store.pool.Exec(ctx,
		`INSERT INTO logger.logs (service, level, message) VALUES ('sprinter', 'info', 'live test')`)
	if err == nil {
		t.Fatal("the read role wrote a log row; it must not be able to")
	}
	// Two different settings can stop the write — a missing INSERT grant, or a
	// role whose transactions are read-only. Either is the containment working,
	// so the test accepts both rather than pinning one deployment's choice.
	refused := strings.ToLower(err.Error())
	if !strings.Contains(refused, "permission denied") && !strings.Contains(refused, "read-only") {
		t.Fatalf("want a permission error, got %v", err)
	}
}

package readstore

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/readstore/readstoretest"
)

// These run against the real read-only role, because the grants are the whole
// point of this package and no compiler checks them. Set
// SPRINTER_TEST_READ_DATABASE_URL; without it the tests skip. The tests that
// assert on rows also need SPRINTER_TEST_OWNER_DATABASE_URL to seed them
// (readstoretest).
//
//	SPRINTER_TEST_READ_DATABASE_URL='postgresql://sprinter_reader:change-me-reader@127.0.0.1:15433/biotron' \
//	  go test ./internal/readstore/ -run Live -v
//
// Nothing here writes through the read role. It could not if it tried, which
// is what the last test proves.
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

func TestLiveHealthHistoryReadsTheSeededChecks(t *testing.T) {
	store, ctx := liveStore(t)
	seeded := readstoretest.Seed(t)
	rows, err := store.HealthHistory(ctx, seeded.Service, time.Now().Add(-24*time.Hour), 200)
	if err != nil {
		t.Fatalf("health history: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the two seeded checks", rows)
	}
	// Newest first: the failure half an hour ago, then the success before it.
	if rows[0].OK || rows[0].Detail != "connection refused" || !rows[0].CheckedAt.Equal(seeded.DownAt) {
		t.Fatalf("newest = %+v", rows[0])
	}
	if !rows[1].OK || !rows[1].CheckedAt.Equal(seeded.UpAt) {
		t.Fatalf("oldest = %+v", rows[1])
	}
	none, err := store.HealthHistory(ctx, seeded.Service, time.Now().Add(-10*time.Minute), 200)
	if err != nil || len(none) != 0 {
		t.Fatalf("checks before since must be left out: %+v, %v", none, err)
	}
}

func TestLiveAccessReportsTheSeededRoster(t *testing.T) {
	store, ctx := liveStore(t)
	seeded := readstoretest.Seed(t)
	report, err := store.Access(ctx, "sprinter")
	if err != nil {
		t.Fatalf("access: %v", err)
	}
	if !report.AppFound || report.AppID != "sprinter" {
		t.Fatalf("report = %+v", report)
	}
	var grant *GrantRow
	for i := range report.Grants {
		if report.Grants[i].Login == seeded.Member {
			grant = &report.Grants[i]
		}
	}
	if grant == nil {
		t.Fatalf("grants = %+v, want %s", report.Grants, seeded.Member)
	}
	if grant.PermissionKey != "admin" || grant.Name != "Live Member" || !grant.IsBanned ||
		grant.IsManager || !grant.GrantedAt.Equal(readstoretest.GrantedAt) {
		t.Fatalf("grant = %+v", *grant)
	}
	if !listed(report.Managers, seeded.Manager) || listed(report.Managers, seeded.Member) {
		t.Fatalf("managers = %+v, want %s and not %s", report.Managers, seeded.Manager, seeded.Member)
	}
	if listed(report.Superusers, seeded.Manager) {
		t.Fatalf("superusers = %+v, a manager is not one", report.Superusers)
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

func listed(operators []OperatorRow, login string) bool {
	for _, operator := range operators {
		if operator.Login == login {
			return true
		}
	}
	return false
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
}

func TestLiveReadRoleCannotWrite(t *testing.T) {
	store, ctx := liveStore(t)
	_, err := store.pool.Exec(ctx,
		`INSERT INTO logger.health_checks (service, ok, checked_at) VALUES ('sprinter', true, now())`)
	if err == nil {
		t.Fatal("the read role wrote a health check; it must not be able to")
	}
	// Two different settings can stop the write — a missing INSERT grant, or a
	// role whose transactions are read-only. Either is the containment working,
	// so the test accepts both rather than pinning one deployment's choice.
	refused := strings.ToLower(err.Error())
	if !strings.Contains(refused, "permission denied") && !strings.Contains(refused, "read-only") {
		t.Fatalf("want a permission error, got %v", err)
	}
}

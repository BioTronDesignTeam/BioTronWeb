package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

// These exercise the store against a real PostgreSQL, because what they pin,
// which rows each query returns and in what order, cannot be observed from Go
// alone. Set LOGGER_TEST_DATABASE_URL to a throwaway database with the logger
// schema migrated; without it the suite skips. The address looks like this:
//
//	postgresql://user:pass@host:5432/db?schema=logger (a placeholder; trufflehog:ignore)
//
// Export the variable first. `-e NAME` with no value passes it through, which
// also keeps the password out of the command line.
//
//	docker run --rm -v "$PWD:/w" -w /w/apps/logger/backend \
//	  -e LOGGER_TEST_DATABASE_URL \
//	  biotron-dev:local go test ./internal/store/ -run Live -v

func liveStore(t *testing.T) (*Store, context.Context, string) {
	t.Helper()
	databaseURL := os.Getenv("LOGGER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("LOGGER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)

	// Each test owns a service id of its own, so the database can be shared
	// with other work and the tests never see each other's rows.
	service := fmt.Sprintf("live-test-%d", time.Now().UnixNano())
	clean := func() {
		if _, err := store.db.Exec(ctx, `DELETE FROM health_checks WHERE service = $1`, service); err != nil {
			t.Errorf("clean health checks: %v", err)
		}
	}
	t.Cleanup(clean)
	return store, ctx, service
}

func TestLiveLatestHealthFallsBackToPostgres(t *testing.T) {
	store, ctx, service := liveStore(t)
	base := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	if err := store.InsertHealth(ctx, model.Health{Service: service, OK: false, Detail: "dial tcp: refused", CheckedAt: base}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertHealth(ctx, model.Health{Service: service, OK: true, CheckedAt: base.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}

	// The monitor has not probed it yet, so the latest row comes from
	// Postgres, with a NULL detail read back as empty.
	latest, err := store.LatestHealth(ctx, []string{service})
	if err != nil {
		t.Fatal(err)
	}
	got := latest[service]
	if !got.OK || got.Detail != "" || !got.CheckedAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("latest = %#v", got)
	}

	// Once the monitor has a reading, it wins over the stored row.
	probed := model.Health{Service: service, OK: false, Detail: "HTTP 503 in 2ms", CheckedAt: base.Add(2 * time.Minute)}
	store.SetLatestHealth(probed)
	latest, err = store.LatestHealth(ctx, []string{service})
	if err != nil {
		t.Fatal(err)
	}
	if latest[service] != probed {
		t.Fatalf("latest = %#v, want the monitor's reading %#v", latest[service], probed)
	}

	var detail *string
	if err := store.db.QueryRow(ctx, `SELECT detail FROM health_checks WHERE service = $1 AND ok`, service).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if detail != nil {
		t.Fatalf("empty detail stored as %q, want NULL", *detail)
	}
}

func TestLiveHealthHistoryCollapsesRuns(t *testing.T) {
	store, ctx, service := liveStore(t)
	base := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	tolerance := 15 * time.Minute
	insert := func(ok bool, at time.Time) {
		t.Helper()
		if err := store.InsertHealth(ctx, model.Health{Service: service, OK: ok, CheckedAt: at}); err != nil {
			t.Fatal(err)
		}
	}

	insert(true, base.Add(-time.Hour)) // before the window: the carry-in
	insert(false, base.Add(-30*time.Minute))
	for minute := 5; minute <= 30; minute += 5 { // one unbroken up run
		insert(true, base.Add(time.Duration(minute)*time.Minute))
	}
	insert(true, base.Add(90*time.Minute)) // after an hour of silence
	insert(false, base.Add(95*time.Minute))

	history, err := store.HealthHistory(ctx, []string{service}, base, base.Add(2*time.Hour), tolerance)
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, point := range history[service] {
		got += fmt.Sprintf("%s ok=%t continuous=%t\n", point.CheckedAt.UTC().Format("15:04"), point.OK, point.Continuous)
	}
	want := "" +
		"11:30 ok=false continuous=false\n" + // carry-in, never continuous
		"12:05 ok=true continuous=true\n" + // a transition
		"12:30 ok=true continuous=false\n" + // starts the silence
		"13:30 ok=true continuous=true\n" + // ends it
		"13:35 ok=false continuous=false\n" // the last row
	if got != want {
		t.Fatalf("history =\n%swant\n%s", got, want)
	}
}

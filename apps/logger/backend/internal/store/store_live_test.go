package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

// These exercise the store against a real PostgreSQL and Redis, because what
// they pin, which rows each query returns and in what order, cannot be
// observed from Go alone. Set LOGGER_TEST_DATABASE_URL to a throwaway database
// with the logger schema migrated, and LOGGER_TEST_REDIS_URL to a throwaway
// Redis; without both the suite skips. The addresses look like this:
//
//	postgresql://user:pass@host:5432/db?schema=logger (a placeholder; trufflehog:ignore)
//	redis://host:6379/15
//
// Export the variables first. `-e NAME` with no value passes each through,
// which also keeps the password out of the command line.
//
//	docker run --rm -v "$PWD:/w" -w /w/apps/logger/backend \
//	  -e LOGGER_TEST_DATABASE_URL -e LOGGER_TEST_REDIS_URL \
//	  biotron-dev:local go test ./internal/store/ -run Live -v

func liveStore(t *testing.T) (*Store, context.Context, string) {
	t.Helper()
	databaseURL := os.Getenv("LOGGER_TEST_DATABASE_URL")
	redisURL := os.Getenv("LOGGER_TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("LOGGER_TEST_DATABASE_URL or LOGGER_TEST_REDIS_URL is not set")
	}
	ctx := context.Background()
	store, err := New(ctx, databaseURL, redisURL, 500)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)

	// Each test owns a service id of its own, so the database can be shared
	// with other work and the tests never see each other's rows.
	service := fmt.Sprintf("live-test-%d", time.Now().UnixNano())
	clean := func() {
		if _, err := store.db.Exec(ctx, `DELETE FROM logs WHERE service = $1`, service); err != nil {
			t.Errorf("clean logs: %v", err)
		}
		if _, err := store.db.Exec(ctx, `DELETE FROM health_checks WHERE service = $1`, service); err != nil {
			t.Errorf("clean health checks: %v", err)
		}
		if err := store.cache.Del(ctx, logKey(service), healthKey(service)).Err(); err != nil {
			t.Errorf("clean cache: %v", err)
		}
	}
	t.Cleanup(clean)
	return store, ctx, service
}

func TestLiveInsertLogRoundTrip(t *testing.T) {
	store, ctx, service := liveStore(t)

	entry, err := store.InsertLog(ctx, model.NewLog{
		Service: service, Level: model.LevelWarning, Message: "disk filling",
		Payload: json.RawMessage(`{"used": 91}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID == "" || entry.Service != service || entry.Level != model.LevelWarning || entry.Message != "disk filling" {
		t.Fatalf("entry = %#v", entry)
	}
	if string(entry.Payload) != `{"used": 91}` || entry.CreatedAt.IsZero() {
		t.Fatalf("payload %s, created_at %v", entry.Payload, entry.CreatedAt)
	}

	// A JSON null payload is stored as SQL NULL, so the row carries none.
	bare, err := store.InsertLog(ctx, model.NewLog{Service: service, Level: model.LevelInfo, Message: "bare", Payload: json.RawMessage(`null`)})
	if err != nil {
		t.Fatal(err)
	}
	if bare.Payload != nil {
		t.Fatalf("null payload came back as %q", bare.Payload)
	}

	// The insert also lands on the Redis tail, newest first.
	recent, err := store.RecentLogs(ctx, []string{service}, nil, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].ID != bare.ID || recent[1].ID != entry.ID {
		t.Fatalf("recent = %#v", recent)
	}
}

func TestLiveQueryLogsFiltersAndPages(t *testing.T) {
	store, ctx, service := liveStore(t)
	base := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	rows := []struct {
		level   string
		message string
		payload string
		at      time.Time
	}{
		{"info", "boot", "", base},
		{"warning", "slow probe", `{"component": "exo-api"}`, base.Add(time.Minute)},
		{"error", "probe failed", `{"component": "calendar-api"}`, base.Add(2 * time.Minute)},
		{"debug", "tick", "", base.Add(3 * time.Minute)},
		{"info", "recovered", `{"component": "Calendar-API"}`, base.Add(4 * time.Minute)},
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		var payload any
		if row.payload != "" {
			payload = row.payload
		}
		var id int64
		if err := store.db.QueryRow(ctx, `
			INSERT INTO logs (service, level, message, payload, created_at)
			VALUES ($1, $2::log_level, $3, $4::jsonb, $5) RETURNING id
		`, service, row.level, row.message, payload, row.at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[i] = fmt.Sprint(id)
	}

	query := func(q model.HistoryQuery) []string {
		t.Helper()
		q.Services = []string{service}
		page, err := store.QueryLogs(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(page.Logs))
		for _, entry := range page.Logs {
			got = append(got, entry.ID)
		}
		return got
	}
	expect := func(name string, got []string, want ...string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	expect("all", query(model.HistoryQuery{}), ids[4], ids[3], ids[2], ids[1], ids[0])
	expect("levels", query(model.HistoryQuery{Levels: []model.LogLevel{model.LevelWarning, model.LevelError}}), ids[2], ids[1])
	from, to := base.Add(time.Minute), base.Add(3*time.Minute)
	expect("from", query(model.HistoryQuery{From: &from}), ids[4], ids[3], ids[2], ids[1])
	expect("to", query(model.HistoryQuery{To: &to}), ids[3], ids[2], ids[1], ids[0])
	expect("from and to", query(model.HistoryQuery{From: &from, To: &to}), ids[3], ids[2], ids[1])
	// The search is case-insensitive and reaches into the payload.
	expect("search", query(model.HistoryQuery{Search: "calendar-api"}), ids[4], ids[2])
	expect("search message", query(model.HistoryQuery{Search: "PROBE"}), ids[2], ids[1])
	expect("other service", func() []string {
		page, err := store.QueryLogs(ctx, model.HistoryQuery{Services: []string{service + "-none"}})
		if err != nil {
			t.Fatal(err)
		}
		return make([]string, len(page.Logs))
	}())

	// Two rows a page: the cursor walks the rest without repeating a row.
	var walked []string
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		page, err := store.QueryLogs(ctx, model.HistoryQuery{Services: []string{service}, Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Logs {
			walked = append(walked, entry.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	expect("paged", walked, ids[4], ids[3], ids[2], ids[1], ids[0])

	if _, err := store.QueryLogs(ctx, model.HistoryQuery{Services: []string{service}, Cursor: "not-a-cursor"}); err == nil || err.Error() != "invalid cursor" {
		t.Fatalf("bad cursor error = %v", err)
	}
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

	// Nothing in Redis yet, so the latest row comes from Postgres, with a NULL
	// detail read back as empty, and is then cached.
	latest, err := store.LatestHealth(ctx, []string{service})
	if err != nil {
		t.Fatal(err)
	}
	got := latest[service]
	if !got.OK || got.Detail != "" || !got.CheckedAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("latest = %#v", got)
	}
	if n, err := store.cache.Exists(ctx, healthKey(service)).Result(); err != nil || n != 1 {
		t.Fatalf("cached = %d, %v", n, err)
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

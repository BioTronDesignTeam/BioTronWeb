package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

// This drives the real route, against a real database, on a fixture clock, so
// the "upcoming" contract is pinned end to end rather than only over the pure
// filter. Set CALENDAR_TEST_DATABASE_URL to a throwaway database with the
// calendar schema migrated; without it the test skips.
//
//	docker run --rm -v "$PWD/backend:/src" -w /src \
//	  -e CALENDAR_TEST_DATABASE_URL='postgresql://user:pass@host:5432/db?schema=calendar' \
//	  golang:1.25-bookworm go test ./internal/server/ -run Live -v

const (
	liveTeamScopeID = "00000000-0000-4000-8000-000000000001"
	liveUIDPrefix   = "live-upcoming-"
)

// fixturePool is a direct connection used only to plant and remove this test's
// own rows, so nothing test-shaped has to be added to the store itself.
func fixturePool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	schema := query.Get("schema")
	if schema == "" {
		schema = "calendar"
	}
	query.Del("schema")
	parsed.RawQuery = query.Encode()
	poolConfig, err := pgxpool.ParseConfig(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func liveUpcomingApp(t *testing.T, now time.Time) (*fiber.App, *store.Store, *pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("CALENDAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALENDAR_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	calendarStore, err := store.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(calendarStore.Close)
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{
		store:    calendarStore,
		config:   config.Config{MaxRangeDays: 370, DefaultTimezone: "America/Toronto"},
		location: location,
		now:      func() time.Time { return now },
	}
	app := fiber.New()
	app.Get("/v1/events/upcoming", handler.listUpcoming)
	app.Get("/v1/events", handler.listOccurrences)
	return app, calendarStore, fixturePool(t, ctx, databaseURL), ctx
}

func liveSeed(t *testing.T, calendarStore *store.Store, ctx context.Context, uid, title, startsAt, endsAt string) {
	t.Helper()
	start, err := calendarlogic.ParseLocalDateTime(startsAt)
	if err != nil {
		t.Fatal(err)
	}
	end, err := calendarlogic.ParseLocalDateTime(endsAt)
	if err != nil {
		t.Fatal(err)
	}
	created, err := calendarStore.CreateSeries(ctx, model.EventSeries{
		UID: liveUIDPrefix + uid, ScopeID: liveTeamScopeID, Title: title,
		StartsAtLocal: start, EndsAtLocal: end, Timezone: "America/Toronto",
	})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	if _, err := calendarStore.PublishSeries(ctx, created.ID, created.Sequence); err != nil {
		t.Fatalf("publish %q: %v", title, err)
	}
}

func liveGet(t *testing.T, app *fiber.App, target string) []model.Occurrence {
	t.Helper()
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, target, nil), fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s returned %d: %s", target, response.StatusCode, body)
	}
	var occurrences []model.Occurrence
	if err := json.Unmarshal(body, &occurrences); err != nil {
		t.Fatalf("decode %s: %v", target, err)
	}
	return occurrences
}

// seededTitles narrows a response to the rows this test planted, so a shared
// database cannot turn an unrelated row into a failure here.
func seededTitles(occurrences []model.Occurrence) []string {
	titles := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if strings.HasPrefix(occurrence.UID, liveUIDPrefix) {
			titles = append(titles, occurrence.Title)
		}
	}
	return titles
}

// An occurrence that has ended must never be served, however recently it
// finished and however long it ran, and one still in progress must be.
func TestLiveUpcomingNeverServesAnEndedOccurrence(t *testing.T) {
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, time.FixedZone("EDT", -4*3600))
	app, calendarStore, pool, ctx := liveUpcomingApp(t, now)

	clean := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM event_series WHERE uid LIKE $1`, liveUIDPrefix+"%"); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)

	liveSeed(t, calendarStore, ctx, "a", "Ended two days ago", "2026-09-01T09:00:00", "2026-09-01T10:00:00")
	liveSeed(t, calendarStore, ctx, "b", "Ended a minute ago", "2026-09-03T21:00:00", "2026-09-03T22:14:00")
	liveSeed(t, calendarStore, ctx, "c", "Four-day build that finished", "2026-08-28T09:00:00", "2026-09-01T17:00:00")
	liveSeed(t, calendarStore, ctx, "d", "In progress right now", "2026-09-03T21:00:00", "2026-09-04T02:00:00")
	liveSeed(t, calendarStore, ctx, "e", "Tomorrow evening", "2026-09-04T18:00:00", "2026-09-04T19:00:00")

	upcoming := liveGet(t, app, "/v1/events/upcoming?limit=20")
	for _, occurrence := range upcoming {
		if !occurrence.EndsAt.After(now) {
			t.Fatalf("%q ended at %s, before the request at %s", occurrence.Title, occurrence.EndsAt, now)
		}
	}
	if got := seededTitles(upcoming); !reflect.DeepEqual(got, []string{"In progress right now", "Tomorrow evening"}) {
		t.Fatalf("upcoming returned %v", got)
	}

	// GET /v1/events answers for an explicit date range the caller asked for, so
	// a past day it asked about must still come back populated. Asserting that
	// here stops the two contracts being "fixed" into each other.
	grid := liveGet(t, app, "/v1/events?from=2026-09-01&to=2026-09-05")
	if got := seededTitles(grid); len(got) != 5 {
		t.Fatalf("a requested date range must return its past days too, got %v", got)
	}
}

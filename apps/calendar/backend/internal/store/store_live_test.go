package store

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

// These exercise the store against a real PostgreSQL, because the two things
// they pin — how much SQL narrowing is safe, and that resetting an occurrence
// removes its row — cannot be observed from Go alone. Set
// CALENDAR_TEST_DATABASE_URL to a throwaway database with the calendar schema
// migrated; without it the suite skips.
//
//	docker run --rm -v "$PWD/backend:/src" -w /src \
//	  -e CALENDAR_TEST_DATABASE_URL='postgresql://user:pass@host:5432/db?schema=calendar' \
//	  golang:1.25-bookworm go test ./internal/store/ -run Live -v

const (
	testProjectID = "0f0f0f0f-0000-4000-8000-00000000f001"
	teamScopeID   = "00000000-0000-4000-8000-000000000001"
)

func liveStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("CALENDAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALENDAR_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)
	return store, ctx
}

// resetFixtures removes only the rows these tests own, so the database can be
// shared with other work.
func resetFixtures(t *testing.T, store *Store, ctx context.Context) {
	t.Helper()
	clean := func() {
		if _, err := store.pool.Exec(ctx, `
			DELETE FROM event_series WHERE scope_id = $1::uuid
			   OR scope_id IN (SELECT id FROM calendar_scopes WHERE parent_id = $1::uuid)
		`, testProjectID); err != nil {
			t.Fatalf("clean series: %v", err)
		}
		// Children first: the parent FK is ON DELETE RESTRICT by design.
		if _, err := store.pool.Exec(ctx, `DELETE FROM calendar_scopes WHERE parent_id = $1::uuid`, testProjectID); err != nil {
			t.Fatalf("clean child scopes: %v", err)
		}
		if _, err := store.pool.Exec(ctx, `DELETE FROM calendar_scopes WHERE id = $1::uuid`, testProjectID); err != nil {
			t.Fatalf("clean scope: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO calendar_scopes (id, kind, name, slug, parent_id)
		VALUES ($1::uuid, 'PROJECT', 'Narrowing fixture', 'narrowing-fixture', $2::uuid)
	`, testProjectID, teamScopeID); err != nil {
		t.Fatalf("create fixture scope: %v", err)
	}
}

func insertSeries(t *testing.T, store *Store, ctx context.Context, id, title, startsAt, endsAt, until string) {
	t.Helper()
	start, err := calendarlogic.ParseLocalDateTime(startsAt)
	if err != nil {
		t.Fatal(err)
	}
	end, err := calendarlogic.ParseLocalDateTime(endsAt)
	if err != nil {
		t.Fatal(err)
	}
	var recurrenceUntil *time.Time
	if until != "" {
		parsed, err := calendarlogic.ParseLocalDate(until)
		if err != nil {
			t.Fatal(err)
		}
		recurrenceUntil = &parsed
	}
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO event_series (id, uid, scope_id, state, title, starts_at_local, ends_at_local, recurrence_until)
		VALUES ($1::uuid, $2, $3::uuid, 'PUBLISHED', $4, $5, $6, $7)
	`, id, id+"@biotron.ca", testProjectID, title, start, end, recurrenceUntil); err != nil {
		t.Fatalf("insert series %s: %v", title, err)
	}
}

func insertOverride(t *testing.T, store *Store, ctx context.Context, id, seriesID, recurrenceID, state, patch string) {
	t.Helper()
	key, err := calendarlogic.ParseLocalDateTime(recurrenceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO event_overrides (id, series_id, recurrence_id_local, state, patch, sequence)
		VALUES ($1::uuid, $2::uuid, $3, $4::"EventOverrideState", $5::jsonb, 1)
	`, id, seriesID, key, state, patch); err != nil {
		t.Fatalf("insert override: %v", err)
	}
}

func titlesOf(series []model.EventSeries) []string {
	titles := make([]string, 0, len(series))
	for _, event := range series {
		titles = append(titles, event.Title)
	}
	return titles
}

// A windowed read must load only what it can render, and must still render
// exactly what an unwindowed read would have.
func TestLiveWindowNarrowsSeriesAndOverrideLoads(t *testing.T) {
	store, ctx := liveStore(t)
	resetFixtures(t, store, ctx)

	const current = "0f0f0f0f-0000-4000-8000-00000000e003"
	insertSeries(t, store, ctx, "0f0f0f0f-0000-4000-8000-00000000e001", "Ancient one-off", "2026-01-05T18:00:00", "2026-01-05T19:00:00", "")
	insertSeries(t, store, ctx, "0f0f0f0f-0000-4000-8000-00000000e002", "Expired weekly", "2026-01-05T18:00:00", "2026-01-05T19:00:00", "2026-02-16")
	insertSeries(t, store, ctx, current, "Current weekly", "2026-09-07T18:00:00", "2026-09-07T19:00:00", "2026-12-14")
	insertSeries(t, store, ctx, "0f0f0f0f-0000-4000-8000-00000000e004", "Next year", "2027-05-03T18:00:00", "2027-05-03T19:00:00", "")

	// Inside the window, and the reason a September read must see it.
	insertOverride(t, store, ctx, "0f0f0f0f-0000-4000-8000-00000000d001", current, "2026-09-14T18:00:00", "CANCELLED", `{}`)
	// Far outside the window and unable to reach it: must not be loaded.
	insertOverride(t, store, ctx, "0f0f0f0f-0000-4000-8000-00000000d002", current, "2026-12-07T18:00:00", "CANCELLED", `{}`)
	// Outside the window but dragged into it, so it must still be loaded.
	insertOverride(t, store, ctx, "0f0f0f0f-0000-4000-8000-00000000d003", current, "2026-11-30T18:00:00", "MODIFIED",
		`{"starts_at_local":"2026-09-21T20:00:00","ends_at_local":"2026-09-21T21:30:00"}`)

	from, _ := calendarlogic.ParseLocalDateTime("2026-09-01T00:00:00")
	to, _ := calendarlogic.ParseLocalDateTime("2026-10-01T00:00:00")

	windowed, err := store.ListSeries(ctx, testProjectID, false, &Window{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(windowed); !reflect.DeepEqual(got, []string{"Current weekly"}) {
		t.Fatalf("a September window loaded %v; only the series that can produce an occurrence in it should be read", got)
	}
	if len(windowed[0].Overrides) != 2 {
		t.Fatalf("expected the in-window override and the one moved into the window, got %d: %+v",
			len(windowed[0].Overrides), windowed[0].Overrides)
	}
	for _, override := range windowed[0].Overrides {
		if calendarlogic.FormatLocal(override.RecurrenceIDLocal) == "2026-12-07T18:00:00" {
			t.Fatal("a December override was loaded to answer a September read")
		}
	}

	full, err := store.ListSeries(ctx, testProjectID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 4 {
		t.Fatalf("an unwindowed read must still load every series, got %v", titlesOf(full))
	}
	if len(full[2].Overrides)+len(full[0].Overrides)+len(full[1].Overrides)+len(full[3].Overrides) != 3 {
		t.Fatal("an unwindowed read must still load every override")
	}

	// The point of the whole change: fewer rows, identical output.
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	windowStart := calendarlogic.InLocation(from, location)
	windowEnd := calendarlogic.InLocation(to, location)
	narrowed, err := calendarlogic.Expand(windowed, windowStart, windowEnd, location)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := calendarlogic.Expand(full, windowStart, windowEnd, location)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(narrowed, complete) {
		t.Fatalf("narrowing changed the answer.\nwindowed: %+v\nfull:     %+v", narrowed, complete)
	}
	if len(narrowed) == 0 {
		t.Fatal("the fixture produced no occurrences, so the comparison proved nothing")
	}
	// The cancelled 14 September occurrence is gone and the far-moved one has
	// arrived on 21 September, which is only true if both were loaded.
	var days []int
	for _, occurrence := range narrowed {
		days = append(days, occurrence.StartsAt.Day())
	}
	if !reflect.DeepEqual(days, []int{7, 21, 21, 28}) {
		t.Fatalf("unexpected September occurrences: %v", days)
	}
}

// Resetting an occurrence has to delete its override row. A retained row used
// to freeze the series against every structural edit, with nothing left in the
// editor to remove.
func TestLiveResetOverrideDeletesTheRow(t *testing.T) {
	store, ctx := liveStore(t)
	resetFixtures(t, store, ctx)

	const seriesID = "0f0f0f0f-0000-4000-8000-00000000e010"
	const controlID = "0f0f0f0f-0000-4000-8000-00000000e011"
	insertSeries(t, store, ctx, seriesID, "Reset subject", "2026-09-07T18:00:00", "2026-09-07T19:00:00", "2026-09-28")
	insertSeries(t, store, ctx, controlID, "Reset subject", "2026-09-07T18:00:00", "2026-09-07T19:00:00", "2026-09-28")

	before, err := store.GetSeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	recurrenceID, _ := calendarlogic.ParseLocalDateTime("2026-09-14T18:00:00")
	if _, err := store.UpsertOverride(ctx, model.EventOverride{
		SeriesID: seriesID, RecurrenceIDLocal: recurrenceID, State: model.OverrideModified,
		Patch: []byte(`{"title":"Moved to the machine shop"}`),
	}, before.Sequence); err != nil {
		t.Fatalf("upsert override: %v", err)
	}

	changed, err := store.GetSeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calendarlogic.OccurrenceChanges(changed.Overrides)) != 1 {
		t.Fatalf("expected one occurrence change, got %+v", changed.Overrides)
	}

	if err := store.ResetOverride(ctx, seriesID, recurrenceID, changed.Sequence); err != nil {
		t.Fatalf("reset: %v", err)
	}

	var rows int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM event_overrides WHERE series_id = $1::uuid`, seriesID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("reset left %d override row(s) behind", rows)
	}

	after, err := store.GetSeries(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Overrides) != 0 {
		t.Fatalf("reset left overrides on the series: %+v", after.Overrides)
	}
	if len(calendarlogic.OccurrenceChanges(after.Overrides)) != 0 {
		t.Fatal("a reset series must not still be blocked from structural edits")
	}

	// Indistinguishable from a series that was never touched.
	control, err := store.GetSeries(ctx, controlID)
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation("America/Toronto")
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, location)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, location)
	resetOccurrences, err := calendarlogic.Expand([]model.EventSeries{after}, from, to, location)
	if err != nil {
		t.Fatal(err)
	}
	controlOccurrences, err := calendarlogic.Expand([]model.EventSeries{control}, from, to, location)
	if err != nil {
		t.Fatal(err)
	}
	if len(resetOccurrences) != len(controlOccurrences) || len(resetOccurrences) != 4 {
		t.Fatalf("expected four weekly occurrences on both, got %d and %d", len(resetOccurrences), len(controlOccurrences))
	}
	for i := range resetOccurrences {
		reset, untouched := resetOccurrences[i], controlOccurrences[i]
		reset.SeriesID, reset.UID, reset.SeriesSequence = "", "", 0
		untouched.SeriesID, untouched.UID, untouched.SeriesSequence = "", "", 0
		if !reflect.DeepEqual(reset, untouched) {
			t.Fatalf("occurrence %d differs from an untouched series:\nreset:     %+v\nuntouched: %+v", i, reset, untouched)
		}
	}
}

package server

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

func TestClampQueryClampsInsteadOfRejecting(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: upcomingDefaultLimit},
		{raw: "not a number", want: upcomingDefaultLimit},
		{raw: "0", want: 1},
		{raw: "-4", want: 1},
		{raw: " 7 ", want: 7},
		{raw: "20", want: upcomingMaxLimit},
		{raw: "5000", want: upcomingMaxLimit},
	}
	for _, test := range tests {
		if got := clampQuery(test.raw, upcomingDefaultLimit, 1, upcomingMaxLimit); got != test.want {
			t.Fatalf("clampQuery(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
	if got := clampQuery("91", upcomingDefaultDays, 1, upcomingMaxDays); got != upcomingMaxDays {
		t.Fatalf("clampQuery days = %d, want %d", got, upcomingMaxDays)
	}
}

// GET /v1/events/upcoming is a published contract for the site and the Sprinter
// bot, so its objects must stay identical to the ones GET /v1/events returns.
func TestUpcomingUsesTheSharedOccurrenceShape(t *testing.T) {
	encoded, err := json.Marshal(model.Occurrence{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	var got []string
	for name := range fields {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{
		"all_day", "description", "ends_at", "location", "modified", "recurrence_id_local",
		"recurring", "scope_id", "scope_kind", "scope_name", "series_id", "series_sequence",
		"starts_at", "timezone", "title", "uid", "url",
	}
	if len(got) != len(want) {
		t.Fatalf("occurrence fields changed: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("occurrence fields changed: %v, want %v", got, want)
		}
	}
}

// The three cases the contract turns on, against a fixture clock so the test
// cannot rot: an occurrence that has ENDED is excluded however recently or
// however long it ran, one IN PROGRESS is included, and one entirely in the
// FUTURE is included.
//
// Asserting only ordering and count would pass while stale events were being
// served, which is exactly how a stale event reached a consumer once.
func TestUpcomingExcludesEndedIncludesInProgressAndFuture(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, location)

	series := []model.EventSeries{
		// ENDED — must never appear.
		newSeries(t, "ended-hours-ago", "Morning standup", "2026-09-03T09:00:00", "2026-09-03T09:30:00"),
		newSeries(t, "ended-a-minute-ago", "Just finished", "2026-09-03T21:00:00", "2026-09-03T22:14:00"),
		newSeries(t, "ended-exactly-now", "Ended on the boundary", "2026-09-03T21:00:00", "2026-09-03T22:15:00"),
		// A long run that began days ago and finished days ago. This is the one
		// that slips through if the lower bound is applied to the start.
		newSeries(t, "long-and-over", "Four-day build", "2026-08-28T09:00:00", "2026-09-01T17:00:00"),
		// A whole day that is already behind us.
		newSeries(t, "all-day-past", "Reading day", "2026-09-02T00:00:00", "2026-09-03T00:00:00"),
		// IN PROGRESS — started before now, ends after now.
		newSeries(t, "in-progress", "Overnight build", "2026-09-03T21:00:00", "2026-09-04T02:00:00"),
		newSeries(t, "long-and-running", "Competition week", "2026-08-31T09:00:00", "2026-09-06T17:00:00"),
		// FUTURE.
		newSeries(t, "starts-exactly-now", "Starting on the boundary", "2026-09-03T22:15:00", "2026-09-03T23:15:00"),
		newSeries(t, "tomorrow", "Controls sync", "2026-09-04T18:00:00", "2026-09-04T19:00:00"),
		// Beyond the window.
		newSeries(t, "beyond-window", "Next term kickoff", "2027-01-11T18:00:00", "2027-01-11T19:00:00"),
	}

	got, err := upcomingOccurrences(series, now, now.AddDate(0, 0, upcomingDefaultDays), upcomingMaxLimit, location)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, occurrence := range got {
		titles = append(titles, occurrence.Title)
		if !occurrence.EndsAt.After(now) {
			t.Fatalf("%q ended at %s, before the %s request, and must not be upcoming",
				occurrence.Title, occurrence.EndsAt, now)
		}
	}
	want := []string{"Competition week", "Overnight build", "Starting on the boundary", "Controls sync"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("upcoming returned %v, want %v", titles, want)
	}

	// Truncation keeps the earliest, and the limit is honoured after expansion.
	limited, err := upcomingOccurrences(series, now, now.AddDate(0, 0, upcomingDefaultDays), 2, location)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Title != "Competition week" || limited[1].Title != "Overnight build" {
		t.Fatalf("truncation kept the wrong occurrences: %+v", limited)
	}
}

// A weekly series must contribute only the instances inside the window, so the
// filter has to run after expansion rather than keeping or dropping the series
// as a whole.
func TestUpcomingFiltersInstancesNotWholeSeries(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, location)
	weekly := newSeries(t, "weekly", "Controls sync", "2026-08-14T18:00:00", "2026-08-14T19:00:00")
	until, err := calendarlogic.ParseLocalDate("2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	weekly.RecurrenceUntil = &until

	got, err := upcomingOccurrences([]model.EventSeries{weekly}, now, now.AddDate(0, 0, upcomingDefaultDays), upcomingMaxLimit, location)
	if err != nil {
		t.Fatal(err)
	}
	var days []string
	for _, occurrence := range got {
		days = append(days, occurrence.StartsAt.Format("2006-01-02"))
	}
	// 14, 21 and 28 August are behind us; 4, 11, 18 and 25 September are not.
	if !reflect.DeepEqual(days, []string{"2026-09-04", "2026-09-11", "2026-09-18", "2026-09-25"}) {
		t.Fatalf("expected only the instances after the request instant, got %v", days)
	}
}

// An empty result must marshal as [] rather than null, because a consumer
// iterating the response should not have to special-case a missing array.
func TestUpcomingReturnsAnEmptyArrayNotNull(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, location)
	got, err := upcomingOccurrences(nil, now, now.AddDate(0, 0, 1), 5, location)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("empty upcoming marshalled as %s", encoded)
	}
}

func TestETagMatchesWeakAndListedValidators(t *testing.T) {
	etag := `"abc123"`
	for _, header := range []string{`"abc123"`, `W/"abc123"`, `"other", "abc123"`, `*`} {
		if !etagMatches(header, etag) {
			t.Fatalf("If-None-Match %q should have matched %s", header, etag)
		}
	}
	for _, header := range []string{`"other"`, `W/"other", "another"`, ``} {
		if etagMatches(header, etag) {
			t.Fatalf("If-None-Match %q should not have matched %s", header, etag)
		}
	}
}

func newSeries(t *testing.T, id, title, startsAt, endsAt string) model.EventSeries {
	t.Helper()
	start, err := calendarlogic.ParseLocalDateTime(startsAt)
	if err != nil {
		t.Fatal(err)
	}
	end, err := calendarlogic.ParseLocalDateTime(endsAt)
	if err != nil {
		t.Fatal(err)
	}
	return model.EventSeries{
		ID: id, UID: id + "@biotron.ca", ScopeID: "scope", ScopeName: "Teamwide", ScopeKind: model.ScopeTeam,
		State: model.EventPublished, Title: title, StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto",
	}
}

func TestRequiresRequestHeader(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{method: "GET", want: false},
		{method: "HEAD", want: false},
		{method: "OPTIONS", want: false},
		{method: "POST", want: true},
		{method: "PATCH", want: true},
		{method: "PUT", want: true},
		{method: "DELETE", want: true},
	}
	for _, test := range tests {
		if got := requiresRequestHeader(test.method); got != test.want {
			t.Fatalf("requiresRequestHeader(%q) = %v, want %v", test.method, got, test.want)
		}
	}
}

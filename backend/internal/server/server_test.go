package server

import (
	"encoding/json"
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

// The upcoming window starts at now, not at the start of today, so a meeting
// that has already finished today must not be returned, and results arrive
// ordered by start and truncated to the limit.
func TestUpcomingWindowStartsAtNowAndTruncates(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, location)
	series := []model.EventSeries{
		newSeries(t, "finished", "Morning standup", "2026-09-07T09:00:00", "2026-09-07T09:30:00"),
		newSeries(t, "running", "All-afternoon build", "2026-09-07T11:00:00", "2026-09-07T17:00:00"),
		newSeries(t, "later", "Controls sync", "2026-09-08T18:00:00", "2026-09-08T19:00:00"),
		newSeries(t, "beyond", "Term review", "2026-11-01T18:00:00", "2026-11-01T19:00:00"),
	}

	occurrences, err := calendarlogic.Expand(series, now, now.AddDate(0, 0, upcomingDefaultDays), location)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, occurrence := range occurrences {
		titles = append(titles, occurrence.Title)
	}
	if len(titles) != 2 || titles[0] != "All-afternoon build" || titles[1] != "Controls sync" {
		t.Fatalf("unexpected upcoming occurrences: %v", titles)
	}
	if limited := occurrences[:1]; limited[0].Title != "All-afternoon build" {
		t.Fatalf("truncation kept the wrong occurrence: %v", limited)
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

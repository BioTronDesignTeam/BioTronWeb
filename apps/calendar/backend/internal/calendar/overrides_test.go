package calendar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
)

// fourWeeks is a published weekly meeting, Tuesdays 18:00 to 19:00 from
// 2026-12-01 to 2026-12-22, with the given overrides keyed by original start.
func fourWeeks(t *testing.T, overrides ...model.EventOverride) model.EventSeries {
	t.Helper()
	start, _ := ParseLocalDateTime("2026-12-01T18:00:00")
	end, _ := ParseLocalDateTime("2026-12-01T19:00:00")
	until, _ := ParseLocalDate("2026-12-22")
	return model.EventSeries{
		ID: "series", UID: "series@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Build night", Location: "E5", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until, Sequence: 6,
		Overrides: overrides,
	}
}

func override(t *testing.T, recurrenceID, state, patch string, sequence int) model.EventOverride {
	t.Helper()
	key, err := ParseLocalDateTime(recurrenceID)
	if err != nil {
		t.Fatal(err)
	}
	return model.EventOverride{
		ID: recurrenceID, RecurrenceIDLocal: key, State: state,
		Patch: json.RawMessage(patch), Sequence: sequence,
	}
}

func expandDecember(t *testing.T, series model.EventSeries) ([]model.Occurrence, *time.Location, error) {
	t.Helper()
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := Expand([]model.EventSeries{series},
		time.Date(2026, 12, 1, 0, 0, 0, 0, location),
		time.Date(2027, 1, 1, 0, 0, 0, 0, location), location)
	return occurrences, location, err
}

func TestExpandSkipsCancelledAndMovesModifiedOccurrences(t *testing.T) {
	series := fourWeeks(t,
		override(t, "2026-12-08T18:00:00", model.OverrideCancelled, `{}`, 1),
		override(t, "2026-12-15T18:00:00", model.OverrideModified, `{"starts_at_local":"2026-12-15T20:00:00","title":"Late"}`, 3),
	)
	occurrences, location, err := expandDecember(t, series)
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 3 {
		t.Fatalf("expected 3 occurrences with week 2 cancelled, got %d: %+v", len(occurrences), occurrences)
	}
	for _, occurrence := range occurrences {
		if occurrence.RecurrenceID == "2026-12-08T18:00:00" {
			t.Fatalf("the cancelled occurrence was served: %+v", occurrence)
		}
	}

	moved := occurrences[1]
	wantStart := time.Date(2026, 12, 15, 20, 0, 0, 0, location)
	if !moved.StartsAt.Equal(wantStart) || !moved.EndsAt.Equal(wantStart.Add(time.Hour)) {
		t.Fatalf("moved occurrence runs %s to %s, want 20:00 to 21:00", moved.StartsAt, moved.EndsAt)
	}
	if moved.Title != "Late" || moved.Location != "E5" || !moved.Modified {
		t.Fatalf("moved occurrence lost its patch or the series' fields: %+v", moved)
	}
	if moved.RecurrenceID != "2026-12-15T18:00:00" || moved.OverrideSequence != 3 || moved.SeriesSequence != 6 {
		t.Fatalf("moved occurrence lost its identity: %+v", moved)
	}

	// The untouched weeks are plain series occurrences.
	for _, untouched := range []model.Occurrence{occurrences[0], occurrences[2]} {
		if untouched.Modified || untouched.OverrideSequence != 0 || untouched.Title != "Build night" {
			t.Fatalf("an untouched occurrence carries override data: %+v", untouched)
		}
	}
}

// A stored override that ends before it starts can only come from a bad row,
// and Expand must say so rather than serve a negative-length meeting.
func TestExpandRefusesAnOverrideThatEndsBeforeItStarts(t *testing.T) {
	series := fourWeeks(t,
		override(t, "2026-12-15T18:00:00", model.OverrideModified, `{"ends_at_local":"2026-12-15T17:00:00"}`, 1),
	)
	_, _, err := expandDecember(t, series)
	if err == nil || !strings.Contains(err.Error(), "ends before it starts") {
		t.Fatalf("Expand() error = %v, want an ends-before-it-starts error", err)
	}
}

// Drafts and cancelled series have no public occurrences.
func TestExpandServesOnlyPublishedSeries(t *testing.T) {
	for _, state := range []string{model.EventDraft, model.EventCancelled} {
		series := fourWeeks(t)
		series.State = state
		occurrences, _, err := expandDecember(t, series)
		if err != nil || len(occurrences) != 0 {
			t.Fatalf("%s series expanded to %d occurrences (err %v)", state, len(occurrences), err)
		}
	}
}

func TestIsGeneratedStart(t *testing.T) {
	series := fourWeeks(t)
	for value, want := range map[string]bool{
		"2026-12-01T18:00:00": true,
		"2026-12-22T18:00:00": true,
		"2026-12-29T18:00:00": false, // past the end date
		"2026-12-08T18:30:00": false, // not the series' time
		"2026-12-09T18:00:00": false, // not the series' weekday
		"2026-11-24T18:00:00": false, // before the first occurrence
	} {
		candidate, _ := ParseLocalDateTime(value)
		if got := IsGeneratedStart(series, candidate); got != want {
			t.Fatalf("IsGeneratedStart(%s) = %v, want %v", value, got, want)
		}
	}
	broken := fourWeeks(t)
	until, _ := ParseLocalDate("2026-11-01")
	broken.RecurrenceUntil = &until
	first, _ := ParseLocalDateTime("2026-12-01T18:00:00")
	if IsGeneratedStart(broken, first) {
		t.Fatal("a series that ends before it starts has no generated starts")
	}
}

func TestValidatePatch(t *testing.T) {
	refused := map[string]string{
		"unknown key":          `{"colour":"red"}`,
		"cleared start":        `{"starts_at_local":null}`,
		"start not a string":   `{"starts_at_local":5}`,
		"start not a time":     `{"ends_at_local":"tomorrow"}`,
		"one-character title":  `{"title":"A"}`,
		"161-character title":  `{"title":"` + strings.Repeat("a", 161) + `"}`,
		"blank title":          `{"title":"   "}`,
		"javascript url":       `{"url":"javascript:alert(1)"}`,
		"5001-char desc":       `{"description":"` + strings.Repeat("a", 5001) + `"}`,
		"301-char location":    `{"location":"` + strings.Repeat("a", 301) + `"}`,
		"1001-char url":        `{"url":"https://a.example/` + strings.Repeat("a", 1000) + `"}`,
		"title not a string":   `{"title":7}`,
		"not an object":        `["title"]`,
		"recurrence edit":      `{"recurrence_until":"2026-12-01"}`,
		"not json at all":      `{`,
		"an array for a field": `{"location":["E5"]}`,
	}
	for name, patch := range refused {
		if _, err := ValidatePatch(json.RawMessage(patch)); err == nil {
			t.Fatalf("%s: ValidatePatch(%s) was accepted", name, patch)
		}
	}

	accepted := map[string]string{
		"empty body":          ``,
		"empty object":        `{}`,
		"cleared description": `{"description":null}`,
		"cleared url":         `{"url":""}`,
		"5000-char desc":      `{"description":"` + strings.Repeat("a", 5000) + `"}`,
		"moved":               `{"starts_at_local":"2026-12-15T20:00","ends_at_local":"2026-12-15T21:00:00"}`,
	}
	for name, patch := range accepted {
		if _, err := ValidatePatch(json.RawMessage(patch)); err != nil {
			t.Fatalf("%s: ValidatePatch(%s) = %v", name, patch, err)
		}
	}
}

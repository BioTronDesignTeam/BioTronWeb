package calendar

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

func TestExpandWeeklySeriesAcrossDaylightSaving(t *testing.T) {
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	until, _ := ParseLocalDate("2026-11-15")
	start, _ := ParseLocalDateTime("2026-10-25T18:00:00")
	end, _ := ParseLocalDateTime("2026-10-25T19:00:00")
	series := model.EventSeries{
		ID: "series", UID: "series@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Weekly meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until,
	}

	occurrences, err := Expand([]model.EventSeries{series},
		time.Date(2026, 10, 1, 0, 0, 0, 0, location),
		time.Date(2026, 12, 1, 0, 0, 0, 0, location), location)
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 4 {
		t.Fatalf("expected 4 occurrences, got %d", len(occurrences))
	}
	for _, occurrence := range occurrences {
		if occurrence.StartsAt.Hour() != 18 {
			t.Fatalf("wall-clock hour changed at %s", occurrence.StartsAt)
		}
	}
	if occurrences[0].StartsAt.Format("-0700") == occurrences[1].StartsAt.Format("-0700") {
		t.Fatal("expected UTC offset to change after DST")
	}
}

// 2026 transitions in America/Toronto: spring forward Sunday 8 March (02:00 EST
// jumps to 03:00 EDT) and fall back Sunday 1 November (02:00 EDT returns to
// 01:00 EST). The fall-back case is covered above; this covers spring forward.
func TestExpandWeeklySeriesAcrossSpringForward(t *testing.T) {
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	until, _ := ParseLocalDate("2026-03-22")
	start, _ := ParseLocalDateTime("2026-03-01T18:00:00")
	end, _ := ParseLocalDateTime("2026-03-01T19:00:00")
	series := model.EventSeries{
		ID: "series", UID: "series@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Weekly meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until,
	}

	occurrences, err := Expand([]model.EventSeries{series},
		time.Date(2026, 2, 1, 0, 0, 0, 0, location),
		time.Date(2026, 4, 1, 0, 0, 0, 0, location), location)
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 4 {
		t.Fatalf("expected 4 occurrences, got %d", len(occurrences))
	}
	for _, occurrence := range occurrences {
		if occurrence.StartsAt.Hour() != 18 || occurrence.EndsAt.Hour() != 19 {
			t.Fatalf("wall-clock hour changed at %s", occurrence.StartsAt)
		}
	}
	// 1 March is EST, 8 March onwards is EDT.
	if occurrences[0].StartsAt.Format("-0700") != "-0500" || occurrences[1].StartsAt.Format("-0700") != "-0400" {
		t.Fatalf("expected the UTC offset to change over spring forward: %s then %s",
			occurrences[0].StartsAt, occurrences[1].StartsAt)
	}
}

// A nonexistent wall clock is resolved with the offset in force before the gap,
// per RFC 5545 section 3.3.5, so the meeting lands on the first instant after
// the skipped hour rather than an hour earlier than every subscriber sees it.
func TestInLocationMovesNonexistentWallClockForward(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	skipped, _ := ParseLocalDateTime("2026-03-08T02:30:00")

	resolved := InLocation(skipped, location)
	if got := resolved.Format("2006-01-02T15:04:05 -0700 MST"); got != "2026-03-08T03:30:00 -0400 EDT" {
		t.Fatalf("nonexistent 02:30 resolved to %s, want 03:30 EDT", got)
	}
	before, _ := ParseLocalDateTime("2026-03-08T01:30:00")
	if !resolved.After(InLocation(before, location)) {
		t.Fatal("a skipped local time must not resolve earlier than the hour before the gap")
	}
}

// An ambiguous wall clock resolves to the first of its two instants, again per
// RFC 5545 section 3.3.5, matching what a subscriber's client does with the
// same TZID value.
func TestInLocationResolvesAmbiguousWallClockToTheFirstInstant(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	repeated, _ := ParseLocalDateTime("2026-11-01T01:30:00")

	resolved := InLocation(repeated, location)
	if got := resolved.Format("2006-01-02T15:04:05 -0700 MST"); got != "2026-11-01T01:30:00 -0400 EDT" {
		t.Fatalf("ambiguous 01:30 resolved to %s, want the earlier EDT instant", got)
	}
	if resolved.Add(time.Hour).Format("-0700") != "-0500" {
		t.Fatal("expected the repeated hour to be followed by EST")
	}
}

// Every wall clock that does exist must be preserved exactly, including the
// hours either side of both transitions.
func TestInLocationPreservesExistingWallClocks(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	for _, wall := range []string{
		"2026-03-08T01:30:00", "2026-03-08T03:30:00", "2026-03-08T18:00:00",
		"2026-11-01T00:30:00", "2026-11-01T02:30:00", "2026-07-04T12:00:00",
	} {
		value, err := ParseLocalDateTime(wall)
		if err != nil {
			t.Fatal(err)
		}
		if got := FormatLocal(InLocation(value, location)); got != wall {
			t.Fatalf("InLocation(%s) = %s, want the same wall clock", wall, got)
		}
	}
}

func TestExpandAppliesMovedOccurrenceByOriginalStart(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	until, _ := ParseLocalDate("2026-09-30")
	start, _ := ParseLocalDateTime("2026-09-07T18:00:00")
	end, _ := ParseLocalDateTime("2026-09-07T19:00:00")
	recurrenceID, _ := ParseLocalDateTime("2026-09-14T18:00:00")
	series := model.EventSeries{
		ID: "series", UID: "series@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until,
		Overrides: []model.EventOverride{{
			ID: "override", RecurrenceIDLocal: recurrenceID, State: model.OverrideModified,
			Patch: json.RawMessage(`{"starts_at_local":"2026-09-15T20:00:00","ends_at_local":"2026-09-15T21:30:00"}`),
		}},
	}

	occurrences, err := Expand([]model.EventSeries{series},
		time.Date(2026, 9, 15, 19, 0, 0, 0, location),
		time.Date(2026, 9, 15, 22, 0, 0, 0, location), location)
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("expected moved occurrence in range, got %d", len(occurrences))
	}
	if occurrences[0].RecurrenceID != "2026-09-14T18:00:00" || occurrences[0].StartsAt.Day() != 15 {
		t.Fatalf("unexpected moved occurrence: %+v", occurrences[0])
	}
}

func TestValidatePatchRejectsRecurrenceChanges(t *testing.T) {
	if _, err := ValidatePatch(json.RawMessage(`{"recurrence_until":"2026-12-01"}`)); err == nil {
		t.Fatal("expected recurrence patch to be rejected")
	}
}

func TestValidatePatchRejectsUnsafeURL(t *testing.T) {
	if _, err := ValidatePatch(json.RawMessage(`{"url":"javascript:alert(1)"}`)); err == nil {
		t.Fatal("expected unsafe occurrence URL to be rejected")
	}
	if _, err := ValidatePatch(json.RawMessage(`{"url":"https://biotron.ca/events"}`)); err != nil {
		t.Fatalf("expected https occurrence URL to be accepted: %v", err)
	}
}

// Resetting an occurrence deletes its override row, so the occurrence must come
// back as a plain generated one with no override metadata at all. Releases
// before that fix kept an empty MODIFIED patch as a tombstone; those rows still
// exist in deployed databases and must be treated as if they were not there,
// because their mere presence used to block every series-level start, timezone,
// all-day, and recurrence-end change with nothing in the UI left to remove.
func TestExpandIgnoresResetOccurrenceOverrides(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	until, _ := ParseLocalDate("2026-09-14")
	start, _ := ParseLocalDateTime("2026-09-07T18:00:00")
	end, _ := ParseLocalDateTime("2026-09-07T19:00:00")
	recurrenceID, _ := ParseLocalDateTime("2026-09-14T18:00:00")
	base := model.EventSeries{
		ID: "series", UID: "series@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until, Sequence: 3,
	}
	tombstoned := base
	tombstoned.Overrides = []model.EventOverride{{
		ID: "reset", RecurrenceIDLocal: recurrenceID, State: model.OverrideModified,
		Patch: json.RawMessage(`{}`), Sequence: 2,
	}}

	for name, series := range map[string]model.EventSeries{"reset": base, "legacy tombstone": tombstoned} {
		occurrences, err := Expand([]model.EventSeries{series},
			time.Date(2026, 9, 14, 0, 0, 0, 0, location),
			time.Date(2026, 9, 15, 0, 0, 0, 0, location), location)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(occurrences) != 1 || occurrences[0].Modified || occurrences[0].OverrideSequence != 0 {
			t.Fatalf("%s: expected an unchanged series occurrence, got %+v", name, occurrences)
		}
	}
	if len(OccurrenceChanges(tombstoned.Overrides)) != 0 {
		t.Fatal("a reset tombstone must not count as an occurrence change")
	}
}

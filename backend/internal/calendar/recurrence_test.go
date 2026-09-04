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

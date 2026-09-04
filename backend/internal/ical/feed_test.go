package ical

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	calendarlogic "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
)

func TestBuildIncludesFiniteRuleAndCancelledOccurrence(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	start, _ := calendarlogic.ParseLocalDateTime("2026-09-07T18:00:00")
	end, _ := calendarlogic.ParseLocalDateTime("2026-09-07T19:00:00")
	until, _ := calendarlogic.ParseLocalDate("2026-12-01")
	recurrenceID, _ := calendarlogic.ParseLocalDateTime("2026-10-12T18:00:00")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	event := model.EventSeries{
		ID: "event", UID: "event@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Controls meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until, UpdatedAt: now,
		Overrides: []model.EventOverride{{
			ID: "cancelled", RecurrenceIDLocal: recurrenceID, State: model.OverrideCancelled,
			Patch: json.RawMessage(`{}`), Sequence: 1, UpdatedAt: now.Add(time.Hour),
		}},
	}

	feed, err := Build("Exo Controls", "https://calendar.example/feeds/scope.ics", []model.EventSeries{event}, location)
	if err != nil {
		t.Fatal(err)
	}
	text := string(feed.Content)
	for _, expected := range []string{
		"RRULE:FREQ=WEEKLY;UNTIL=20261130T230000Z",
		"RECURRENCE-ID;TZID=America/Toronto:20261012T180000",
		"STATUS:CANCELLED",
		"\r\n",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("feed missing %q:\n%s", expected, text)
		}
	}
	if !feed.LastModified.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected last modified: %s", feed.LastModified)
	}
}

func TestFoldLineUsesContinuation(t *testing.T) {
	line := "DESCRIPTION:" + strings.Repeat("é", 50)
	folded := foldLine(line)
	if !strings.Contains(folded, "\r\n ") {
		t.Fatalf("expected folded line, got %q", folded)
	}
	if !utf8Valid(folded) {
		t.Fatal("folding split a UTF-8 rune")
	}
}

func TestBuildAllowsOccurrenceMovedOutsideOriginalWindow(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	start, _ := calendarlogic.ParseLocalDateTime("2026-09-07T18:00:00")
	end, _ := calendarlogic.ParseLocalDateTime("2026-09-07T19:00:00")
	until, _ := calendarlogic.ParseLocalDate("2026-12-01")
	recurrenceID, _ := calendarlogic.ParseLocalDateTime("2026-09-14T18:00:00")
	event := model.EventSeries{
		ID: "event", UID: "event@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until,
		Overrides: []model.EventOverride{{
			ID: "moved", RecurrenceIDLocal: recurrenceID, State: model.OverrideModified,
			Patch: json.RawMessage(`{"starts_at_local":"2026-11-20T18:00:00","ends_at_local":"2026-11-20T19:00:00"}`),
		}},
	}

	feed, err := Build("Meeting", "", []model.EventSeries{event}, location)
	if err != nil {
		t.Fatal(err)
	}
	text := string(feed.Content)
	if !strings.Contains(text, "RECURRENCE-ID;TZID=America/Toronto:20260914T180000") ||
		!strings.Contains(text, "DTSTART;TZID=America/Toronto:20261120T180000") {
		t.Fatalf("feed does not contain the far-moved occurrence:\n%s", text)
	}
}

func TestBuildCancelsModifiedOccurrencesOfACancelledSeries(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	start, _ := calendarlogic.ParseLocalDateTime("2026-09-07T18:00:00")
	end, _ := calendarlogic.ParseLocalDateTime("2026-09-07T19:00:00")
	until, _ := calendarlogic.ParseLocalDate("2026-12-01")
	recurrenceID, _ := calendarlogic.ParseLocalDateTime("2026-10-12T18:00:00")
	event := model.EventSeries{
		ID: "event", UID: "event@biotron.ca", ScopeID: "scope", State: model.EventCancelled,
		Title: "Controls meeting", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until,
		Overrides: []model.EventOverride{{
			ID: "moved", RecurrenceIDLocal: recurrenceID, State: model.OverrideModified,
			Patch: json.RawMessage(`{"starts_at_local":"2026-10-13T18:00:00","ends_at_local":"2026-10-13T19:00:00"}`),
		}},
	}

	feed, err := Build("Exo Controls", "", []model.EventSeries{event}, location)
	if err != nil {
		t.Fatal(err)
	}
	text := string(feed.Content)
	if strings.Contains(text, "STATUS:CONFIRMED") {
		t.Fatalf("a cancelled series must not ship a confirmed instance:\n%s", text)
	}
	if strings.Count(text, "STATUS:CANCELLED") != 2 {
		t.Fatalf("expected the master and the moved instance to be cancelled:\n%s", text)
	}
	if !strings.Contains(text, "RECURRENCE-ID;TZID=America/Toronto:20261012T180000") {
		t.Fatalf("the cancelled exception lost its recurrence id:\n%s", text)
	}
}

func utf8Valid(value string) bool {
	return strings.ToValidUTF8(value, "invalid") == value
}

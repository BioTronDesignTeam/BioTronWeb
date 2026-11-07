package ical

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	calendarlogic "github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
)

// weeklySeries is a published weekly meeting, 18:00 to 19:00 from 2026-09-07
// to 2026-12-01, with one override.
func weeklySeries(t *testing.T, sequence int, override model.EventOverride) model.EventSeries {
	t.Helper()
	start, _ := calendarlogic.ParseLocalDateTime("2026-09-07T18:00:00")
	end, _ := calendarlogic.ParseLocalDateTime("2026-09-07T19:00:00")
	until, _ := calendarlogic.ParseLocalDate("2026-12-01")
	override.RecurrenceIDLocal, _ = calendarlogic.ParseLocalDateTime("2026-10-12T18:00:00")
	return model.EventSeries{
		ID: "event", UID: "event@biotron.ca", ScopeID: "scope", State: model.EventPublished,
		Title: "Controls meeting", Description: "Weekly sync", Location: "E5 2001",
		URL: "https://example.com/agenda", StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto", RecurrenceUntil: &until, Sequence: sequence,
		UpdatedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Overrides: []model.EventOverride{override},
	}
}

// exception is the unfolded VEVENT of the one changed occurrence in a feed.
func exception(t *testing.T, event model.EventSeries) string {
	t.Helper()
	location, _ := time.LoadLocation("America/Toronto")
	feed, err := Build("Exo", "", []model.EventSeries{event}, location)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(feed.Content), "\r\n ", "")
	for _, block := range strings.Split(text, "BEGIN:VEVENT") {
		if strings.Contains(block, "RECURRENCE-ID") {
			return block
		}
	}
	t.Fatalf("the feed has no exception:\n%s", text)
	return ""
}

func sequenceOf(t *testing.T, block string) int {
	t.Helper()
	for _, line := range strings.Split(block, "\r\n") {
		if value, ok := strings.CutPrefix(line, "SEQUENCE:"); ok {
			sequence, err := strconv.Atoi(value)
			if err != nil {
				t.Fatal(err)
			}
			return sequence
		}
	}
	t.Fatalf("no SEQUENCE in:\n%s", block)
	return 0
}

// SEQUENCE used to be series + override. A reset deletes the override row and
// bumps the series once, so after five edits, a reset and one new edit the sum
// fell from S+5 to S+3, and a client that honours SEQUENCE kept the old
// instance. Each step below is what the store writes.
func TestExceptionSequenceNeverGoesDown(t *testing.T) {
	moved := model.EventOverride{
		ID: "o", State: model.OverrideModified, Patch: json.RawMessage(`{"title":"Moved"}`),
	}
	// Created at series 3 and edited four more times: series 8, override 5.
	moved.Sequence = 5
	before := sequenceOf(t, exception(t, weeklySeries(t, 8, moved)))
	// A reset bumps the series to 9 and deletes the row; the next edit bumps
	// it to 10 and starts a new row at 1.
	moved.Sequence = 1
	after := sequenceOf(t, exception(t, weeklySeries(t, 10, moved)))
	if after <= before {
		t.Fatalf("SEQUENCE went from %d to %d after a reset and re-edit", before, after)
	}
}

// An exception repeats what the series says unless the override changes it,
// and a move that names only the start keeps the series' duration.
func TestExceptionCarriesEveryChangedField(t *testing.T) {
	changed := model.EventOverride{
		ID: "o", State: model.OverrideModified, Sequence: 2,
		UpdatedAt: time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC),
		Patch: json.RawMessage(`{"title":"Late build","description":"Bring, food; and tools",` +
			`"location":"","url":"https://example.com/late","starts_at_local":"2026-10-12T20:00:00"}`),
	}
	block := exception(t, weeklySeries(t, 4, changed))
	for _, want := range []string{
		"RECURRENCE-ID;TZID=America/Toronto:20261012T180000",
		"DTSTART;TZID=America/Toronto:20261012T200000",
		"DTEND;TZID=America/Toronto:20261012T210000",
		"SUMMARY:Late build",
		`DESCRIPTION:Bring\, food\; and tools`,
		"URL:https://example.com/late",
		"LAST-MODIFIED:20261001T093000Z",
		"SEQUENCE:4",
		"STATUS:CONFIRMED",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("exception missing %q:\n%s", want, block)
		}
	}
	// The patch cleared the location, so the series' one must not leak in.
	if strings.Contains(block, "LOCATION:") {
		t.Fatalf("a cleared location is still in the exception:\n%s", block)
	}

	// An explicit end wins over the series' duration.
	changed.Patch = json.RawMessage(`{"starts_at_local":"2026-10-13T08:00:00","ends_at_local":"2026-10-13T12:30:00"}`)
	block = exception(t, weeklySeries(t, 4, changed))
	for _, want := range []string{
		"DTSTART;TZID=America/Toronto:20261013T080000",
		"DTEND;TZID=America/Toronto:20261013T123000",
		"SUMMARY:Controls meeting",
		"LOCATION:E5 2001",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("exception missing %q:\n%s", want, block)
		}
	}
}

// A cancelled occurrence keeps its slot and the series' details, so a client
// can match it to the instance it removes.
func TestCancelledExceptionKeepsItsSlot(t *testing.T) {
	cancelled := model.EventOverride{
		ID: "o", State: model.OverrideCancelled, Sequence: 1, Patch: json.RawMessage(`{}`),
	}
	block := exception(t, weeklySeries(t, 2, cancelled))
	for _, want := range []string{
		"DTSTART;TZID=America/Toronto:20261012T180000",
		"DTEND;TZID=America/Toronto:20261012T190000",
		"SUMMARY:Controls meeting",
		"STATUS:CANCELLED",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("cancelled exception missing %q:\n%s", want, block)
		}
	}
}

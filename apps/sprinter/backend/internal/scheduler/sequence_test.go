package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/calendar"
)

// An announced occurrence (series sequence 5) is then edited on its own:
// Calendar now reports series_sequence 6 and override_sequence 1. The edit has
// to reach the Discord announcement, and so does a later series-wide edit.
func TestAnOccurrenceEditAfterAnnouncingIsPropagated(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := announceAutomation()
	occ := calendar.Occurrence{
		UID: "uid-x", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 5,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()
	s := New(Deps{Store: fs, Calendar: fc, Discord: fd, Location: toronto, Now: fixedNow(now)})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}

	occ.SeriesSequence = 6
	occ.OverrideSequence = 1
	occ.Title = "Kickoff MOVED to 19:00"
	occ.StartsAt = occ.StartsAt.Add(time.Hour)
	fc.occurrences = []calendar.Occurrence{occ}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fd.sentMessages) != 1 || len(fd.editedMessages) != 1 {
		t.Fatalf("sent = %d, edited = %d, want one post and one edit", len(fd.sentMessages), len(fd.editedMessages))
	}

	// A series-wide edit after the override bumps only the series.
	occ.SeriesSequence = 7
	occ.Title = "Kickoff in the new lab"
	fc.occurrences = []calendar.Occurrence{occ}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fd.editedMessages) != 2 {
		t.Fatalf("edited = %d, want the series edit to reach Discord too", len(fd.editedMessages))
	}
}

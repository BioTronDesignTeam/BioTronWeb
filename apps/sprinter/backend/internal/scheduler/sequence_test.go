package scheduler

import (
	"context"
	"strings"
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

// Edit, edit again, reset, re-edit: Calendar bumps the series sequence at
// each step, and the override row restarts at 1 after the reset. Every step
// has to reach the announcement. Series plus override fell at the reset (9 to
// 8) and the reset never reached Discord.
func TestEditResetAndReEditAllReachTheAnnouncement(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := announceAutomation()
	occ := calendar.Occurrence{
		UID: "uid-r", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 5,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()
	s := New(Deps{Store: fs, Calendar: fc, Discord: fd, Location: toronto, Now: fixedNow(now)})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		series, override int
		title            string
	}{
		{6, 1, "Kickoff at 19:00"},
		{7, 2, "Kickoff at 19:30"},
		{8, 0, "Kickoff"},
		{9, 1, "Kickoff in the new lab"},
	}
	for i, step := range steps {
		occ.SeriesSequence, occ.OverrideSequence, occ.Title = step.series, step.override, step.title
		fc.occurrences = []calendar.Occurrence{occ}
		if err := s.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(fd.editedMessages) != i+1 || !strings.Contains(fd.editedMessages[i].content, step.title) {
			t.Fatalf("step %d (%q): edits = %+v", i, step.title, fd.editedMessages)
		}
	}
	if len(fd.sentMessages) != 1 {
		t.Fatalf("sent = %d, want one post edited in place", len(fd.sentMessages))
	}
}

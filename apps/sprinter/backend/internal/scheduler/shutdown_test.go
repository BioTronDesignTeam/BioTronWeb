package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
)

// A shutdown that arrives while Discord is accepting a message must still
// let that message be recorded, or the next run posts it again. And nothing
// further may be sent once it has arrived.
func TestAShutdownMidSendRecordsTheMessageAndSendsNoMore(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := announceAutomation()
	first := calendar.Occurrence{
		UID: "uid-s1", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	second := first
	second.UID = "uid-s2"
	fs := newFakeStore(automation)
	fd := newFakeDiscord()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fd.onSend = cancel

	s := New(Deps{
		Store: fs, Calendar: &fakeCalendar{occurrences: []calendar.Occurrence{first, second}},
		Discord: fd, Location: toronto, Now: fixedNow(now),
	})
	if err := s.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.sentMessages) != 1 {
		t.Fatalf("sent = %d, want 1: nothing after the shutdown", len(fd.sentMessages))
	}
	if _, ok := fs.posted[seenKey(automation.ID, "uid-s1", first.RecurrenceID)]; !ok {
		t.Fatal("the message sent as the shutdown arrived was not recorded")
	}
}

// Run returns once its tick is done, which is what main waits on before it
// closes the store and the Discord session.
func TestRunReturnsAfterTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := New(Deps{Store: newFakeStore(), Calendar: &fakeCalendar{}, Discord: newFakeDiscord()})
	done := make(chan struct{})
	go func() {
		s.Run(ctx, time.Hour)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
}

// A cancellation nudge whose seen row could not be dropped is not sent twice:
// the next tick finds the nudge already recorded.
func TestACancellationNudgeIsSentOnce(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation()
	starts := now.Add(6 * time.Hour)
	fs := newFakeStore(automation)
	seen := store.SeenOccurrence{
		AutomationID: automation.ID, UID: "uid-once", RecurrenceIDLocal: "2026-09-10T18:00:00",
		Sequence: 1, StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff",
	}
	fd := newFakeDiscord()
	s := New(Deps{Store: fs, Calendar: &fakeCalendar{}, Discord: fd, Model: &fakeModel{text: "draft"}, Location: toronto, Now: fixedNow(now)})
	for range 2 {
		fs.seen[seenKey(automation.ID, seen.UID, seen.RecurrenceIDLocal)] = seen
		if err := s.Tick(context.Background()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	if len(fd.dms) != 1 {
		t.Fatalf("dms = %d, want one cancellation nudge", len(fd.dms))
	}
}

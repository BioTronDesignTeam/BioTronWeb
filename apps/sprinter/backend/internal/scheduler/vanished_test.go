package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
)

// stillScheduled is a feed in which uid-v still has its 18:00 occurrence on
// 2026-09-10. /v1/events stopped returning it all the same: its scope was
// archived, it moved to another scope, or an override moved it past the
// fetch window.
const stillScheduled = "BEGIN:VCALENDAR\r\n" +
	"BEGIN:VEVENT\r\nUID:uid-v\r\nDTSTART;TZID=America/Toronto:20260910T180000\r\nSTATUS:CONFIRMED\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:uid-v\r\nRECURRENCE-ID;TZID=America/Toronto:20260910T180000\r\n" +
	"DTSTART;TZID=America/Toronto:20261030T180000\r\nSTATUS:CONFIRMED\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// cancelledOccurrence is the same series with that occurrence cancelled.
const cancelledOccurrence = "BEGIN:VCALENDAR\r\n" +
	"BEGIN:VEVENT\r\nUID:uid-v\r\nDTSTART;TZID=America/Toronto:20260910T180000\r\nSTATUS:CONFIRMED\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:uid-v\r\nRECURRENCE-ID;TZID=America/Toronto:20260910T180000\r\n" +
	"DTSTART;TZID=America/Toronto:20260910T180000\r\nSTATUS:CANCELLED\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

const vanishedRecurrence = "2026-09-10T18:00:00"

// seedVanished records uid-v as seen (and, for an ANNOUNCE, posted) by a
// previous tick, as if Calendar used to return it.
func seedVanished(fs *fakeStore, automation store.Automation, starts time.Time) {
	fs.seen[seenKey(automation.ID, "uid-v", vanishedRecurrence)] = store.SeenOccurrence{
		AutomationID: automation.ID, UID: "uid-v", RecurrenceIDLocal: vanishedRecurrence,
		Sequence: 1, StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff",
	}
	if automation.Kind == store.KindAnnounce {
		fs.posted[seenKey(automation.ID, "uid-v", vanishedRecurrence)] = store.PostedOccurrence{
			AutomationID: automation.ID, UID: "uid-v", RecurrenceIDLocal: vanishedRecurrence,
			Sequence: 1, ChannelID: automation.ChannelID, MessageID: "posted-v",
		}
	}
}

// An occurrence that left /v1/events but is still on the calendar is not a
// cancellation. Nobody is told it was cancelled, and the automation stops
// tracking it until it comes back into view.
func TestAVanishedOccurrenceThatStillExistsIsNotCancelled(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	nudgeAuto := nudgeAutomation()
	announceAuto := announceAutomation()
	fs := newFakeStore(nudgeAuto, announceAuto)
	seedVanished(fs, nudgeAuto, now.Add(6*time.Hour))
	seedVanished(fs, announceAuto, now.Add(6*time.Hour))
	fc := &fakeCalendar{feedBody: stillScheduled}
	fd := newFakeDiscord()

	s := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: &fakeModel{text: "draft"}, Location: toronto, Now: fixedNow(now)})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.dms) != 0 || len(fd.sentMessages) != 0 || len(fd.editedMessages) != 0 {
		t.Fatalf("an occurrence still on the calendar was announced as cancelled: dms=%+v sent=%+v edited=%+v",
			fd.dms, fd.sentMessages, fd.editedMessages)
	}
	if len(fs.seen) != 0 {
		t.Fatalf("seen rows = %+v, want them forgotten", fs.seen)
	}
	if fc.feedCalls != 1 {
		t.Fatalf("feed read %d times, want once for the whole tick", fc.feedCalls)
	}
}

// When the feed marks the occurrence cancelled, both kinds react as before.
func TestAVanishedOccurrenceTheFeedCancelsIsCancelled(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	nudgeAuto := nudgeAutomation()
	announceAuto := announceAutomation()
	fs := newFakeStore(nudgeAuto, announceAuto)
	seedVanished(fs, nudgeAuto, now.Add(6*time.Hour))
	seedVanished(fs, announceAuto, now.Add(6*time.Hour))
	fc := &fakeCalendar{feedBody: cancelledOccurrence}
	fd := newFakeDiscord()

	s := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: &fakeModel{text: "draft"}, Location: toronto, Now: fixedNow(now)})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.dms) != 1 {
		t.Fatalf("dms = %+v, want one cancellation nudge", fd.dms)
	}
	if len(fd.editedMessages) != 1 || !strings.HasPrefix(fd.editedMessages[0].content, "Cancelled: ") {
		t.Fatalf("edits = %+v, want the announcement marked cancelled", fd.editedMessages)
	}
}

// Without the feed nothing can be decided, so nothing is: no message, and
// the seen rows stay for the next tick.
func TestAFeedFailureDecidesNothing(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	announceAuto := announceAutomation()
	fs := newFakeStore(announceAuto)
	seedVanished(fs, announceAuto, now.Add(6*time.Hour))
	fc := &fakeCalendar{feedErr: errors.New("calendar is down")}
	fd := newFakeDiscord()

	s := New(Deps{Store: fs, Calendar: fc, Discord: fd, Location: toronto, Now: fixedNow(now)})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.editedMessages) != 0 {
		t.Fatalf("edits = %+v, want none", fd.editedMessages)
	}
	if _, ok := fs.seen[seenKey(announceAuto.ID, "uid-v", vanishedRecurrence)]; !ok {
		t.Fatal("the seen row was dropped although nothing was decided")
	}
}

// A tick where nothing vanished never reads the feed.
func TestNothingVanishedMeansNoFeedRead(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	fs := newFakeStore(announceAutomation())
	fc := &fakeCalendar{}
	s := New(Deps{Store: fs, Calendar: fc, Discord: newFakeDiscord(), Location: toronto, Now: fixedNow(now)})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if fc.feedCalls != 0 {
		t.Fatalf("feed read %d times", fc.feedCalls)
	}
}

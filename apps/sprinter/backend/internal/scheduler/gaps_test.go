package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

// post_hour holds an announcement back until the local wall clock reaches
// it, however close the occurrence is.
func TestAnnounceWaitsForPostHour(t *testing.T) {
	automation := announceAutomation()
	nine := 9
	automation.PostHour = &nine
	before := time.Date(2026, 9, 10, 8, 59, 0, 0, toronto)
	occ := calendar.Occurrence{
		UID: "uid-ph", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: before.Add(9 * time.Hour), EndsAt: before.Add(10 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()

	clock := before
	s := New(Deps{Store: fs, Calendar: fc, Discord: fd, Location: toronto, Now: func() time.Time { return clock }})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.sentMessages) != 0 {
		t.Fatalf("posted at 08:59 with post_hour 9: %+v", fd.sentMessages)
	}

	clock = time.Date(2026, 9, 10, 9, 0, 0, 0, toronto)
	for range 2 {
		if err := s.Tick(context.Background()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	if len(fd.sentMessages) != 1 {
		t.Fatalf("sent = %d at 09:00, want exactly one post", len(fd.sentMessages))
	}
}

// any_author lets anyone's announcement satisfy the nudge, but not a bot's,
// and not one sent before the lookback window.
func TestAnyAuthorNudgeIsSatisfiedByAHumanNotABot(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	occ := calendar.Occurrence{
		UID: "uid-any", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	run := func(t *testing.T, messages []Message) (*fakeStore, *fakeDiscord) {
		t.Helper()
		automation := nudgeAutomation()
		automation.AnyAuthor = true
		fs := newFakeStore(automation)
		fd := newFakeDiscord()
		fd.channelMessages[automation.ChannelID] = messages
		s := New(Deps{
			Store: fs, Calendar: &fakeCalendar{occurrences: []calendar.Occurrence{occ}},
			Discord: fd, Model: &fakeModel{text: "draft"}, Location: toronto, Now: fixedNow(now),
		})
		if err := s.Tick(context.Background()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
		return fs, fd
	}
	key := nudgeKey("auto-nudge", occ.UID, occ.RecurrenceID, 1, store.TriggerUpcoming)

	t.Run("a bot message does not satisfy it", func(t *testing.T) {
		fs, fd := run(t, []Message{
			{ID: snowflakeAt(now.Add(-time.Hour), 1), AuthorID: "some-bot", IsBot: true, Timestamp: now.Add(-time.Hour)},
		})
		if len(fd.dms) != 1 || fs.nudges[key].Status != store.NudgeSent {
			t.Fatalf("dms = %+v, nudge = %+v, want a SENT nudge", fd.dms, fs.nudges[key])
		}
	})
	t.Run("a human who is not the lead satisfies it", func(t *testing.T) {
		fs, fd := run(t, []Message{
			{ID: snowflakeAt(now.Add(-time.Hour), 1), AuthorID: "another-member", Timestamp: now.Add(-time.Hour)},
		})
		if len(fd.dms) != 0 || fs.nudges[key].Status != store.NudgeSatisfied {
			t.Fatalf("dms = %+v, nudge = %+v, want SATISFIED and no DM", fd.dms, fs.nudges[key])
		}
	})
	t.Run("a message before the lookback does not", func(t *testing.T) {
		old := now.Add(-48 * time.Hour)
		fs, fd := run(t, []Message{
			{ID: snowflakeAt(now.Add(-time.Hour), 1), AuthorID: "another-member", Timestamp: old},
		})
		if len(fd.dms) != 1 || fs.nudges[key].Status != store.NudgeSent {
			t.Fatalf("dms = %+v, nudge = %+v, want a SENT nudge", fd.dms, fs.nudges[key])
		}
	})
}

// One automation that panics is reported and does not stop the next one.
func TestOnePanickingAutomationDoesNotStopTheOthers(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	broken := announceAutomation()
	broken.ID = "auto-broken"
	broken.ScopeID = "scope-broken"
	healthy := announceAutomation()
	occ := calendar.Occurrence{
		UID: "uid-ok", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	events, received := logclienttest.New(t, "sprinter")
	fd := newFakeDiscord()
	s := New(Deps{
		Store:    newFakeStore(broken, healthy),
		Calendar: &fakeCalendar{occurrences: []calendar.Occurrence{occ}, panicScope: broken.ScopeID},
		Discord:  fd, Events: events, Location: toronto, Now: fixedNow(now),
	})
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.sentMessages) != 1 || fd.sentMessages[0].channelOrUser != healthy.ChannelID {
		t.Fatalf("sent = %+v, want the healthy automation's post", fd.sentMessages)
	}

	timeout := time.After(5 * time.Second)
	for {
		select {
		case event := <-received:
			if event.Message != "Automation failed" {
				continue
			}
			message, _ := event.Payload["error"].(string)
			if event.Payload["automation_id"] != broken.ID || !strings.HasPrefix(message, "panic: ") {
				t.Fatalf("event = %+v", event)
			}
			return
		case <-timeout:
			t.Fatal(`no "Automation failed" event for the panic`)
		}
	}
}

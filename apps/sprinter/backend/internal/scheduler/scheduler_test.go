package scheduler

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

var toronto = mustLoadToronto()

func mustLoadToronto() *time.Location {
	location, err := time.LoadLocation("America/Toronto")
	if err != nil {
		panic(err)
	}
	return location
}

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func nudgeAutomation(now time.Time) store.Automation {
	lead := "lead-user"
	return store.Automation{
		ID: "auto-nudge", Kind: store.KindNudge, Name: "nudge",
		ScopeID: "scope-1", ChannelID: "chan-1", LeadUserID: &lead,
		LeadHours: 24, LookbackHours: 24, Deliver: store.DeliverDM, Enabled: true,
	}
}

func announceAutomation() store.Automation {
	return store.Automation{
		ID: "auto-announce", Kind: store.KindAnnounce, Name: "announce",
		ScopeID: "scope-1", ChannelID: "chan-2",
		LeadHours: 24, LookbackHours: 24, Deliver: store.DeliverChannel, Enabled: true,
	}
}

// 1. An upcoming event with no lead message sends a nudge with the draft.
func TestTickNudgeSendsDraftWhenNoLeadMessage(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation(now)
	occ := calendar.Occurrence{
		UID: "uid-1", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord() // no messages in the channel: no one has announced yet
	fm := &fakeModel{text: "Come to Kickoff!"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(fd.dms) != 1 {
		t.Fatalf("dms = %d, want 1", len(fd.dms))
	}
	if fd.dms[0].channelOrUser != "lead-user" {
		t.Fatalf("dm recipient = %q", fd.dms[0].channelOrUser)
	}
	if !strings.Contains(fd.dms[0].content, "Come to Kickoff!") || !strings.Contains(fd.dms[0].content, "Kickoff") {
		t.Fatalf("dm content = %q", fd.dms[0].content)
	}
	if len(fd.sentMessages) != 0 {
		t.Fatalf("sent channel messages = %d, want 0 (a nudge must never post to the announcement channel)", len(fd.sentMessages))
	}
	nudge, ok := fs.nudges[nudgeKey(automation.ID, occ.UID, occ.RecurrenceID, 1, store.TriggerUpcoming)]
	if !ok || nudge.Status != store.NudgeSent {
		t.Fatalf("nudge = %+v, ok=%v", nudge, ok)
	}
}

// 2. With a lead message already in the window, the tick records SATISFIED
// and sends nothing.
func TestTickNudgeSatisfiedWhenLeadAlreadyPosted(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation(now)
	occ := calendar.Occurrence{
		UID: "uid-2", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()
	fd.channelMessages[automation.ChannelID] = []Message{
		{ID: "1", AuthorID: "lead-user", Timestamp: now.Add(-time.Hour)},
	}
	fm := &fakeModel{text: "unused"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(fd.dms) != 0 || len(fd.sentMessages) != 0 {
		t.Fatalf("dms = %d, sent = %d, want 0 and 0", len(fd.dms), len(fd.sentMessages))
	}
	nudge, ok := fs.nudges[nudgeKey(automation.ID, occ.UID, occ.RecurrenceID, 1, store.TriggerUpcoming)]
	if !ok || nudge.Status != store.NudgeSatisfied {
		t.Fatalf("nudge = %+v, ok=%v", nudge, ok)
	}
}

// 3. A vanished future occurrence triggers a CANCELLED nudge (for a NUDGE
// automation) and an announcement edit (for an ANNOUNCE automation that had
// already posted about it).
func TestTickVanishedOccurrenceTriggersCancellation(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	nudgeAuto := nudgeAutomation(now)
	announceAuto := announceAutomation()
	starts := now.Add(6 * time.Hour)

	fs := newFakeStore(nudgeAuto, announceAuto)
	// Seed both automations as if a previous tick already saw the occurrence.
	fs.seen[seenKey(nudgeAuto.ID, "uid-3", "2026-09-10T18:00:00")] = store.SeenOccurrence{
		AutomationID: nudgeAuto.ID, UID: "uid-3", RecurrenceIDLocal: "2026-09-10T18:00:00",
		Sequence: 1, StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff",
	}
	fs.seen[seenKey(announceAuto.ID, "uid-3", "2026-09-10T18:00:00")] = store.SeenOccurrence{
		AutomationID: announceAuto.ID, UID: "uid-3", RecurrenceIDLocal: "2026-09-10T18:00:00",
		Sequence: 1, StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff",
	}
	fs.posted[seenKey(announceAuto.ID, "uid-3", "2026-09-10T18:00:00")] = store.PostedOccurrence{
		AutomationID: announceAuto.ID, UID: "uid-3", RecurrenceIDLocal: "2026-09-10T18:00:00",
		Sequence: 1, ChannelID: announceAuto.ChannelID, MessageID: "posted-1",
	}

	// Calendar's fresh fetch no longer contains the occurrence: it was
	// cancelled.
	fc := &fakeCalendar{occurrences: nil}
	fd := newFakeDiscord()
	fm := &fakeModel{text: "unused"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	nudge, ok := fs.nudges[nudgeKey(nudgeAuto.ID, "uid-3", "2026-09-10T18:00:00", 1, store.TriggerCancelled)]
	if !ok {
		t.Fatal("no CANCELLED nudge recorded")
	}
	if nudge.Status != store.NudgeSent {
		t.Fatalf("nudge.Status = %q, want SENT (nothing satisfied it)", nudge.Status)
	}
	if len(fd.dms) != 1 || !strings.Contains(fd.dms[0].content, "Kickoff") {
		t.Fatalf("dms = %+v", fd.dms)
	}

	if len(fd.editedMessages) != 1 {
		t.Fatalf("edited messages = %d, want 1", len(fd.editedMessages))
	}
	edit := fd.editedMessages[0]
	if edit.messageID != "posted-1" || !strings.HasPrefix(edit.content, "Cancelled: ") {
		t.Fatalf("edit = %+v", edit)
	}

	if _, ok := fs.seen[seenKey(nudgeAuto.ID, "uid-3", "2026-09-10T18:00:00")]; ok {
		t.Fatal("seen row for the nudge automation was not deleted")
	}
	if _, ok := fs.seen[seenKey(announceAuto.ID, "uid-3", "2026-09-10T18:00:00")]; ok {
		t.Fatal("seen row for the announce automation was not deleted")
	}
}

// 4. A Calendar fetch error changes nothing for that automation.
func TestTickCalendarFetchErrorChangesNothing(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation(now)
	fs := newFakeStore(automation)
	preExisting := store.SeenOccurrence{
		AutomationID: automation.ID, UID: "uid-4", RecurrenceIDLocal: "2026-09-10T18:00:00",
		Sequence: 1, StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour), Title: "Kickoff",
	}
	fs.seen[seenKey(automation.ID, "uid-4", "2026-09-10T18:00:00")] = preExisting

	fc := &fakeCalendar{err: context.DeadlineExceeded}
	fd := newFakeDiscord()
	fm := &fakeModel{text: "unused"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(fd.dms) != 0 || len(fd.sentMessages) != 0 || len(fd.editedMessages) != 0 {
		t.Fatalf("discord was touched: dms=%d sent=%d edited=%d", len(fd.dms), len(fd.sentMessages), len(fd.editedMessages))
	}
	if fs.insertNudgeCalls != 0 || fs.upsertPostedCalls != 0 {
		t.Fatalf("store was touched: nudges=%d posted=%d", fs.insertNudgeCalls, fs.upsertPostedCalls)
	}
	got := fs.seen[seenKey(automation.ID, "uid-4", "2026-09-10T18:00:00")]
	if got != preExisting {
		t.Fatalf("seen row changed: %+v vs %+v", got, preExisting)
	}
}

// 5. An announcement is posted once and edited when the sequence rises.
func TestTickAnnouncementPostedOnceThenEditedOnSequenceBump(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := announceAutomation()
	occ := calendar.Occurrence{
		UID: "uid-5", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", Location: "Lab", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	if len(fd.sentMessages) != 1 {
		t.Fatalf("sent = %d, want 1", len(fd.sentMessages))
	}
	firstMessageID := fd.sentMessages[0].messageID

	// The occurrence was edited: its sequence rose.
	occ.SeriesSequence = 2
	occ.Title = "Kickoff (moved room)"
	fc.occurrences = []calendar.Occurrence{occ}
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick: %v", err)
	}

	if len(fd.sentMessages) != 1 {
		t.Fatalf("sent after edit = %d, want still 1 (an edit, not a repost)", len(fd.sentMessages))
	}
	if len(fd.editedMessages) != 1 {
		t.Fatalf("edited = %d, want 1", len(fd.editedMessages))
	}
	edit := fd.editedMessages[0]
	if edit.messageID != firstMessageID || !strings.Contains(edit.content, "Kickoff (moved room)") {
		t.Fatalf("edit = %+v", edit)
	}
	posted := fs.posted[seenKey(automation.ID, occ.UID, occ.RecurrenceID)]
	if posted.Sequence != 2 {
		t.Fatalf("posted.Sequence = %d, want 2", posted.Sequence)
	}
}

// 6. A second tick, with nothing having changed since the first, repeats
// none of the above: no repeated posts, edits, DMs, or nudges.
func TestTickSecondRunRepeatsNothing(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	nudgeAuto := nudgeAutomation(now)
	announceAuto := announceAutomation()
	nudgeOcc := calendar.Occurrence{
		UID: "uid-6n", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Standup", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(6*time.Hour + 30*time.Minute),
	}
	announceOcc := calendar.Occurrence{
		UID: "uid-6a", RecurrenceID: "2026-09-10T19:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(7 * time.Hour), EndsAt: now.Add(8 * time.Hour),
	}

	fs := newFakeStore(nudgeAuto, announceAuto)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{nudgeOcc, announceOcc}}
	fd := newFakeDiscord()
	fm := &fakeModel{text: "draft"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick: %v", err)
	}

	dms, sent, edited := len(fd.dms), len(fd.sentMessages), len(fd.editedMessages)
	nudges, posted := fs.insertNudgeCalls, fs.upsertPostedCalls
	if dms == 0 || sent == 0 {
		t.Fatalf("first tick did not do the expected work: dms=%d sent=%d", dms, sent)
	}

	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick: %v", err)
	}

	if len(fd.dms) != dms || len(fd.sentMessages) != sent || len(fd.editedMessages) != edited {
		t.Fatalf("second tick repeated Discord activity: dms %d->%d sent %d->%d edited %d->%d",
			dms, len(fd.dms), sent, len(fd.sentMessages), edited, len(fd.editedMessages))
	}
	if fs.insertNudgeCalls != nudges || fs.upsertPostedCalls != posted {
		t.Fatalf("second tick repeated store writes: nudges %d->%d posted %d->%d",
			nudges, fs.insertNudgeCalls, posted, fs.upsertPostedCalls)
	}
}

func TestSnowflakeAfter(t *testing.T) {
	// 2026-09-10T00:00:00Z is a known instant; the formula is Discord's own
	// (ms since the Discord epoch) << 22.
	when := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	want := strconv.FormatUint(uint64(when.UnixMilli()-discordEpochMillis)<<22, 10)
	got := snowflakeAfter(when)
	if got != want {
		t.Fatalf("snowflakeAfter(%v) = %s, want %s", when, got, want)
	}
}

func TestSnowflakeAfterClampsBeforeTheDiscordEpoch(t *testing.T) {
	got := snowflakeAfter(time.Unix(0, 0))
	if got != "0" {
		t.Fatalf("snowflakeAfter(pre-epoch) = %s, want 0", got)
	}
}

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
		{ID: snowflakeAt(now.Add(-time.Hour), 1), AuthorID: "lead-user", Timestamp: now.Add(-time.Hour)},
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

// The lead's message sits behind two full pages of other people's chatter.
// A walk that only read the first page, or that combined before and after,
// would miss it and nudge a lead who had already announced.
func TestNudgeFindsTheLeadBehindTwoPagesOfChatter(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, toronto)
	automation := nudgeAutomation(now)
	occ := calendar.Occurrence{
		UID: "uid-page", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "Kickoff", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	fs := newFakeStore(automation)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()
	var history []Message
	// Oldest first: a message from the lead before the window that must not
	// count, then two full pages of chatter, then the lead's announcement as
	// the newest message, which a forward walk reaches only on page three.
	history = append(history, Message{ID: snowflakeAt(now.Add(-30*time.Hour), 1), AuthorID: "lead-user", Timestamp: now.Add(-30 * time.Hour)})
	for i := 0; i < 2*channelHistoryPageLimit; i++ {
		at := now.Add(-20*time.Hour + time.Duration(i)*time.Minute)
		history = append(history, Message{ID: snowflakeAt(at, i+2), AuthorID: "someone-else", Timestamp: at})
	}
	history = append(history, Message{ID: snowflakeAt(now.Add(-time.Hour), 1), AuthorID: "lead-user", Timestamp: now.Add(-time.Hour)})
	fd.channelMessages[automation.ChannelID] = history
	fm := &fakeModel{text: "unused"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(fd.dms) != 0 || len(fd.sentMessages) != 0 {
		t.Fatalf("dms = %d, sent = %d, want 0 and 0: the lead's message was missed", len(fd.dms), len(fd.sentMessages))
	}
	if fd.historyCalls < 3 {
		t.Fatalf("historyCalls = %d, want at least 3 pages", fd.historyCalls)
	}
}

// A short lead_hours must not shrink the window the cancellation check
// compares over. Before this, an automation with lead_hours 24 fetched only
// the next 25 hours, so every seen occurrence further out read as vanished
// and the whole week was announced as cancelled at once.
func TestTickDoesNotCancelOccurrencesBeyondTheLeadWindow(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation(now)
	starts := now.Add(5 * 24 * time.Hour) // five days out, far past lead_hours 24
	occ := calendar.Occurrence{
		UID: "uid-far", RecurrenceID: "2026-09-15T18:00:00", SeriesSequence: 1,
		Title: "Competition", StartsAt: starts, EndsAt: starts.Add(2 * time.Hour),
	}
	fs := newFakeStore(automation)
	fs.seen[seenKey(automation.ID, occ.UID, occ.RecurrenceID)] = store.SeenOccurrence{
		AutomationID: automation.ID, UID: occ.UID, RecurrenceIDLocal: occ.RecurrenceID,
		Sequence: 1, StartsAt: starts, EndsAt: occ.EndsAt, Title: occ.Title,
	}
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{occ}}
	fd := newFakeDiscord()
	fm := &fakeModel{text: "unused"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	// The fetch has to reach the occurrence, or the comparison is over a
	// window the seen row was never in.
	if fc.to.Sub(fc.from) != fetchWindow {
		t.Fatalf("fetch window = %v, want %v", fc.to.Sub(fc.from), fetchWindow)
	}
	if len(fd.dms) != 0 || len(fd.sentMessages) != 0 || len(fd.editedMessages) != 0 {
		t.Fatalf("an occurrence that is still on the calendar was announced as cancelled: dms=%+v sent=%+v edited=%+v",
			fd.dms, fd.sentMessages, fd.editedMessages)
	}
	if _, ok := fs.nudges[nudgeKey(automation.ID, occ.UID, occ.RecurrenceID, 1, store.TriggerCancelled)]; ok {
		t.Fatal("a CANCELLED nudge was recorded for an occurrence Calendar still returns")
	}
	if _, ok := fs.seen[seenKey(automation.ID, occ.UID, occ.RecurrenceID)]; !ok {
		t.Fatal("the seen row was deleted for an occurrence Calendar still returns")
	}
	// It is still too far out to nudge about, so nothing UPCOMING either.
	if _, ok := fs.nudges[nudgeKey(automation.ID, occ.UID, occ.RecurrenceID, 1, store.TriggerUpcoming)]; ok {
		t.Fatal("an occurrence five days out was nudged with lead_hours 24")
	}
}

// The other half of the same rule: inside the window, a seen occurrence that
// stopped coming back is still a cancellation.
func TestTickStillCancelsInsideTheFetchWindow(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation(now)
	starts := now.Add(5 * 24 * time.Hour)
	fs := newFakeStore(automation)
	fs.seen[seenKey(automation.ID, "uid-gone", "2026-09-15T18:00:00")] = store.SeenOccurrence{
		AutomationID: automation.ID, UID: "uid-gone", RecurrenceIDLocal: "2026-09-15T18:00:00",
		Sequence: 1, StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Competition",
	}
	// A seen row past the fetch window says nothing: it was never asked for.
	beyond := now.Add(fetchWindow + time.Hour)
	fs.seen[seenKey(automation.ID, "uid-beyond", "2026-09-18T18:00:00")] = store.SeenOccurrence{
		AutomationID: automation.ID, UID: "uid-beyond", RecurrenceIDLocal: "2026-09-18T18:00:00",
		Sequence: 1, StartsAt: beyond, EndsAt: beyond.Add(time.Hour), Title: "Later",
	}
	fc := &fakeCalendar{occurrences: nil}
	fd := newFakeDiscord()
	fm := &fakeModel{text: "Competition is cancelled."}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if _, ok := fs.nudges[nudgeKey(automation.ID, "uid-gone", "2026-09-15T18:00:00", 1, store.TriggerCancelled)]; !ok {
		t.Fatal("an occurrence that vanished inside the window must still be a cancellation")
	}
	if _, ok := fs.nudges[nudgeKey(automation.ID, "uid-beyond", "2026-09-18T18:00:00", 1, store.TriggerCancelled)]; ok {
		t.Fatal("an occurrence starting past the fetch window must not be read as cancelled")
	}
	if _, ok := fs.seen[seenKey(automation.ID, "uid-beyond", "2026-09-18T18:00:00")]; !ok {
		t.Fatal("the seen row past the window was deleted")
	}
}

// A cancellation is news the channel has not had. The lead's own earlier
// announcement sits inside the lookback window and used to satisfy the
// nudge, which silenced the only message that could correct it.
func TestTickCancelledNudgeIgnoresTheEarlierAnnouncement(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	automation := nudgeAutomation(now)
	starts := now.Add(6 * time.Hour)
	fs := newFakeStore(automation)
	fs.seen[seenKey(automation.ID, "uid-c", "2026-09-10T18:00:00")] = store.SeenOccurrence{
		AutomationID: automation.ID, UID: "uid-c", RecurrenceIDLocal: "2026-09-10T18:00:00",
		Sequence: 1, StartsAt: starts, EndsAt: starts.Add(time.Hour), Title: "Kickoff",
	}
	fc := &fakeCalendar{occurrences: nil}
	fd := newFakeDiscord()
	// The lead announced the event an hour ago, well inside the lookback.
	fd.channelMessages[automation.ChannelID] = []Message{
		{ID: snowflakeAt(now.Add(-time.Hour), 1), AuthorID: "lead-user", Timestamp: now.Add(-time.Hour)},
	}
	fm := &fakeModel{text: "Kickoff is cancelled."}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	nudge, ok := fs.nudges[nudgeKey(automation.ID, "uid-c", "2026-09-10T18:00:00", 1, store.TriggerCancelled)]
	if !ok || nudge.Status != store.NudgeSent {
		t.Fatalf("nudge = %+v, ok = %v, want a SENT cancellation nudge", nudge, ok)
	}
	if len(fd.dms) != 1 || !strings.Contains(fd.dms[0].content, "Kickoff is cancelled.") {
		t.Fatalf("dms = %+v", fd.dms)
	}
}

// Nothing the bot posts may ping a room. The nudge that mentions the lead is
// the one exception, and it names that one id.
func TestOutgoingMessagesPingOnlyTheLead(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	nudgeAuto := nudgeAutomation(now)
	nudgeAuto.Deliver = store.DeliverChannel
	announceAuto := announceAutomation()
	nudgeOcc := calendar.Occurrence{
		UID: "uid-m1", RecurrenceID: "2026-09-10T18:00:00", SeriesSequence: 1,
		Title: "@everyone Standup", StartsAt: now.Add(6 * time.Hour), EndsAt: now.Add(7 * time.Hour),
	}
	announceOcc := calendar.Occurrence{
		UID: "uid-m2", RecurrenceID: "2026-09-10T19:00:00", SeriesSequence: 1,
		Title: "@everyone Kickoff", StartsAt: now.Add(7 * time.Hour), EndsAt: now.Add(8 * time.Hour),
	}
	fs := newFakeStore(nudgeAuto, announceAuto)
	fc := &fakeCalendar{occurrences: []calendar.Occurrence{nudgeOcc, announceOcc}}
	fd := newFakeDiscord()
	fm := &fakeModel{text: "draft"}

	scheduler := New(Deps{Store: fs, Calendar: fc, Discord: fd, Model: fm, Location: toronto, Now: fixedNow(now)})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	// Both automations watch the same scope, so each occurrence produces a
	// nudge in one channel and an announcement in the other.
	if len(fd.sentMessages) != 4 {
		t.Fatalf("sent = %d, want 4 (two nudges, two announcements)", len(fd.sentMessages))
	}
	for _, sent := range fd.sentMessages {
		switch sent.channelOrUser {
		case nudgeAuto.ChannelID:
			if len(sent.mentionUsers) != 1 || sent.mentionUsers[0] != *nudgeAuto.LeadUserID {
				t.Fatalf("the nudge must ping the lead and nobody else, got %v", sent.mentionUsers)
			}
		case announceAuto.ChannelID:
			if len(sent.mentionUsers) != 0 {
				t.Fatalf("an announcement must ping nobody, got %v", sent.mentionUsers)
			}
		default:
			t.Fatalf("message to an unexpected channel %q", sent.channelOrUser)
		}
	}
}

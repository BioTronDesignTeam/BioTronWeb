package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// fakeStore is the in-memory Store the scheduler tests drive. It is not
// safe for concurrent use; Tick runs automations one at a time in these
// tests, so that is never needed.
type fakeStore struct {
	automations []store.Automation
	seen        map[string]store.SeenOccurrence
	posted      map[string]store.PostedOccurrence
	nudges      map[string]store.Nudge

	upsertPostedCalls int
	insertNudgeCalls  int
}

func newFakeStore(automations ...store.Automation) *fakeStore {
	return &fakeStore{
		automations: automations,
		seen:        map[string]store.SeenOccurrence{},
		posted:      map[string]store.PostedOccurrence{},
		nudges:      map[string]store.Nudge{},
	}
}

func seenKey(automationID, uid, recurrenceIDLocal string) string {
	return automationID + "|" + uid + "|" + recurrenceIDLocal
}

func nudgeKey(automationID, uid, recurrenceIDLocal string, sequence int, trigger string) string {
	return fmt.Sprintf("%s|%s|%s|%d|%s", automationID, uid, recurrenceIDLocal, sequence, trigger)
}

func (f *fakeStore) ListEnabledAutomations(context.Context) ([]store.Automation, error) {
	return f.automations, nil
}

func (f *fakeStore) UpsertSeen(_ context.Context, automationID string, occ store.SeenOccurrence) (store.SeenOccurrence, error) {
	occ.AutomationID = automationID
	key := seenKey(automationID, occ.UID, occ.RecurrenceIDLocal)
	if existing, ok := f.seen[key]; ok {
		occ.FirstSeenAt = existing.FirstSeenAt
	}
	f.seen[key] = occ
	return occ, nil
}

func (f *fakeStore) ListSeenFuture(_ context.Context, automationID string, now time.Time) ([]store.SeenOccurrence, error) {
	future := []store.SeenOccurrence{}
	for _, occ := range f.seen {
		if occ.AutomationID == automationID && occ.StartsAt.After(now) {
			future = append(future, occ)
		}
	}
	return future, nil
}

func (f *fakeStore) DeleteSeen(_ context.Context, automationID, uid, recurrenceIDLocal string) error {
	key := seenKey(automationID, uid, recurrenceIDLocal)
	if _, ok := f.seen[key]; !ok {
		return store.ErrNotFound
	}
	delete(f.seen, key)
	return nil
}

func (f *fakeStore) GetPosted(_ context.Context, automationID, uid, recurrenceIDLocal string) (store.PostedOccurrence, error) {
	posted, ok := f.posted[seenKey(automationID, uid, recurrenceIDLocal)]
	if !ok {
		return store.PostedOccurrence{}, store.ErrNotFound
	}
	return posted, nil
}

func (f *fakeStore) UpsertPosted(_ context.Context, automationID, uid, recurrenceIDLocal string, sequence int, channelID, messageID string) (store.PostedOccurrence, error) {
	f.upsertPostedCalls++
	posted := store.PostedOccurrence{
		AutomationID: automationID, UID: uid, RecurrenceIDLocal: recurrenceIDLocal,
		Sequence: sequence, ChannelID: channelID, MessageID: messageID,
	}
	f.posted[seenKey(automationID, uid, recurrenceIDLocal)] = posted
	return posted, nil
}

func (f *fakeStore) HasNudge(_ context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger string) (bool, error) {
	_, ok := f.nudges[nudgeKey(automationID, uid, recurrenceIDLocal, sequence, trigger)]
	return ok, nil
}

func (f *fakeStore) InsertNudge(_ context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger, status string, messageID *string) (store.Nudge, error) {
	key := nudgeKey(automationID, uid, recurrenceIDLocal, sequence, trigger)
	if _, ok := f.nudges[key]; ok {
		return store.Nudge{}, store.ErrConflict
	}
	f.insertNudgeCalls++
	nudge := store.Nudge{
		AutomationID: automationID, UID: uid, RecurrenceIDLocal: recurrenceIDLocal,
		Sequence: sequence, Trigger: trigger, Status: status, MessageID: messageID,
	}
	f.nudges[key] = nudge
	return nudge, nil
}

// fakeCalendar answers every Occurrences call the same way: with occurrences,
// or with err when set.
type fakeCalendar struct {
	occurrences []calendar.Occurrence
	err         error
	calls       int
	// from and to record the last window asked for, so a test can pin the
	// span the cancellation check compares over.
	from, to time.Time
}

func (f *fakeCalendar) Occurrences(_ context.Context, _ string, from, to time.Time) ([]calendar.Occurrence, error) {
	f.calls++
	f.from, f.to = from, to
	if f.err != nil {
		return nil, f.err
	}
	return f.occurrences, nil
}

// fakeDiscord records every call the scheduler makes and answers
// ChannelMessages with a fixed, pre-seeded slice of Message.
type fakeDiscord struct {
	channelMessages map[string][]Message
	nextID          int
	historyCalls    int

	sentMessages   []sentMessage
	editedMessages []editedMessage
	dms            []sentMessage
}

type sentMessage struct {
	channelOrUser string
	content       string
	messageID     string
	mentionUsers  []string
}

type editedMessage struct {
	channelID, messageID, content string
}

func newFakeDiscord() *fakeDiscord {
	return &fakeDiscord{channelMessages: map[string][]Message{}}
}

// ChannelMessages pages the way Discord does: with afterID set it returns the
// limit messages just after that id, newest first. beforeID is refused, so a
// call that combines the two, which Discord treats as mutually exclusive,
// fails a test instead of passing by accident.
func (f *fakeDiscord) ChannelMessages(channelID string, limit int, beforeID, afterID string) ([]Message, error) {
	f.historyCalls++
	if beforeID != "" {
		return nil, fmt.Errorf("fake ChannelMessages: beforeID is not supported")
	}
	var window []Message
	for _, message := range f.channelMessages[channelID] {
		if afterID == "" || snowflakeLess(afterID, message.ID) {
			window = append(window, message)
		}
	}
	sort.Slice(window, func(i, j int) bool { return snowflakeLess(window[i].ID, window[j].ID) })
	if len(window) > limit {
		window = window[:limit]
	}
	for i, j := 0, len(window)-1; i < j; i, j = i+1, j-1 {
		window[i], window[j] = window[j], window[i]
	}
	return window, nil
}

// snowflakeAt builds a plausible message id for a message sent at t, so the
// fake can page by id the way Discord does.
func snowflakeAt(t time.Time, n int) string {
	millis := t.UnixMilli() - discordEpochMillis
	return strconv.FormatUint(uint64(millis)<<22|uint64(n), 10)
}

func (f *fakeDiscord) SendMessage(channelID, content string, mentionUsers []string) (string, error) {
	f.nextID++
	id := fmt.Sprintf("msg-%d", f.nextID)
	f.sentMessages = append(f.sentMessages, sentMessage{
		channelOrUser: channelID, content: content, messageID: id, mentionUsers: mentionUsers,
	})
	return id, nil
}

func (f *fakeDiscord) EditMessage(channelID, messageID, content string) error {
	f.editedMessages = append(f.editedMessages, editedMessage{channelID: channelID, messageID: messageID, content: content})
	return nil
}

func (f *fakeDiscord) DirectMessage(userID, content string) (string, error) {
	f.nextID++
	id := fmt.Sprintf("dm-%d", f.nextID)
	f.dms = append(f.dms, sentMessage{channelOrUser: userID, content: content, messageID: id})
	return id, nil
}

// fakeModel implements model.Model with a fixed answer or a fixed error.
type fakeModel struct {
	text string
	err  error
}

func (f *fakeModel) Name() string { return "fake" }

func (f *fakeModel) Generate(_ context.Context, _ string, _ []model.Message, _ []model.ToolSpec) (model.Response, error) {
	if f.err != nil {
		return model.Response{}, f.err
	}
	return model.Response{Text: f.text}, nil
}

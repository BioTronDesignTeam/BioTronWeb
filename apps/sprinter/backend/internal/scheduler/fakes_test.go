package scheduler

import (
	"context"
	"fmt"
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
}

func (f *fakeCalendar) Occurrences(context.Context, string, time.Time, time.Time) ([]calendar.Occurrence, error) {
	f.calls++
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

	sentMessages   []sentMessage
	editedMessages []editedMessage
	dms            []sentMessage
}

type sentMessage struct {
	channelOrUser string
	content       string
	messageID     string
}

type editedMessage struct {
	channelID, messageID, content string
}

func newFakeDiscord() *fakeDiscord {
	return &fakeDiscord{channelMessages: map[string][]Message{}}
}

func (f *fakeDiscord) ChannelMessages(channelID string, limit int, beforeID, afterID string) ([]Message, error) {
	return f.channelMessages[channelID], nil
}

func (f *fakeDiscord) SendMessage(channelID, content string) (string, error) {
	f.nextID++
	id := fmt.Sprintf("msg-%d", f.nextID)
	f.sentMessages = append(f.sentMessages, sentMessage{channelOrUser: channelID, content: content, messageID: id})
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

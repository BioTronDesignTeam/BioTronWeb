package store

import (
	"context"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store/queries"
)

// The nudge triggers and statuses, spelled as the enum values the migration
// created.
const (
	TriggerUpcoming  = "UPCOMING"
	TriggerCancelled = "CANCELLED"

	NudgeSatisfied = "SATISFIED"
	NudgeSent      = "SENT"
)

// SeenOccurrence is the last fetch's record of one occurrence, keyed by the
// automation and the occurrence's own identity. The scheduler compares this
// against Calendar's current answer to notice a vanished occurrence, since
// /v1/events drops a cancelled one instead of flagging it.
type SeenOccurrence struct {
	AutomationID      string    `json:"automation_id"`
	UID               string    `json:"uid"`
	RecurrenceIDLocal string    `json:"recurrence_id_local"`
	Sequence          int       `json:"sequence"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
	Title             string    `json:"title"`
	ScopeID           string    `json:"scope_id"`
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
}

// PostedOccurrence is the ANNOUNCE message the bot posted for one occurrence.
// Sequence is the occurrence's sequence at the time of posting, so a later
// edit to the occurrence is told apart from one already announced.
type PostedOccurrence struct {
	AutomationID      string    `json:"automation_id"`
	UID               string    `json:"uid"`
	RecurrenceIDLocal string    `json:"recurrence_id_local"`
	Sequence          int       `json:"sequence"`
	ChannelID         string    `json:"channel_id"`
	MessageID         string    `json:"message_id"`
	PostedAt          time.Time `json:"posted_at"`
}

// Nudge is one lead nudge sent, or found already satisfied, for one
// occurrence at one sequence. The primary key includes Trigger so an
// UPCOMING nudge and a later CANCELLED nudge for the same occurrence are
// distinct rows, and includes Sequence so an edited occurrence can be
// nudged again.
type Nudge struct {
	AutomationID      string    `json:"automation_id"`
	UID               string    `json:"uid"`
	RecurrenceIDLocal string    `json:"recurrence_id_local"`
	Sequence          int       `json:"sequence"`
	Trigger           string    `json:"trigger"`
	Status            string    `json:"status"`
	MessageID         *string   `json:"message_id"`
	CreatedAt         time.Time `json:"created_at"`
}

// ListEnabledAutomations is what the scheduler ticks over. Disabled
// automations are skipped entirely, not merely left alone once ticked.
func (s *Store) ListEnabledAutomations(ctx context.Context) ([]Automation, error) {
	rows, err := s.q.ListEnabledAutomations(ctx)
	if err != nil {
		return nil, err
	}
	automations := make([]Automation, 0, len(rows))
	for _, row := range rows {
		automations = append(automations, automationFrom(queries.GetAutomationRow(row)))
	}
	return automations, nil
}

// UpsertSeen records that automationID's fetch found occ just now.
// FirstSeenAt is set once, on the row's first insert; LastSeenAt moves on
// every call, which is what lets ListSeenFuture tell a still-fetched
// occurrence from one that stopped coming back.
func (s *Store) UpsertSeen(ctx context.Context, automationID string, occ SeenOccurrence) (SeenOccurrence, error) {
	var counts int4s
	params := queries.UpsertSeenParams{
		AutomationID:      automationID,
		Uid:               occ.UID,
		RecurrenceIDLocal: occ.RecurrenceIDLocal,
		Sequence:          counts.of("sequence", occ.Sequence),
		StartsAt:          occ.StartsAt,
		EndsAt:            occ.EndsAt,
		Title:             occ.Title,
		ScopeID:           occ.ScopeID,
	}
	if counts.err != nil {
		return SeenOccurrence{}, counts.err
	}
	row, err := s.q.UpsertSeen(ctx, params)
	if err != nil {
		return SeenOccurrence{}, mapConstraintError(mapNotFound(err))
	}
	return seenFrom(row), nil
}

// ListSeenFuture returns every occurrence automationID last saw whose
// starts_at is still ahead of now. Anything already past is irrelevant to
// cancellation detection: an event Calendar stops returning because it has
// already happened is not a cancellation.
func (s *Store) ListSeenFuture(ctx context.Context, automationID string, now time.Time) ([]SeenOccurrence, error) {
	rows, err := s.q.ListSeenFuture(ctx, queries.ListSeenFutureParams{
		AutomationID: automationID,
		Now:          now,
	})
	if err != nil {
		return nil, err
	}
	seen := make([]SeenOccurrence, 0, len(rows))
	for _, row := range rows {
		seen = append(seen, seenFrom(row))
	}
	return seen, nil
}

// DeleteSeen forgets one occurrence, once the scheduler has finished
// reacting to its cancellation.
func (s *Store) DeleteSeen(ctx context.Context, automationID, uid, recurrenceIDLocal string) error {
	deleted, err := s.q.DeleteSeen(ctx, queries.DeleteSeenParams{
		AutomationID:      automationID,
		Uid:               uid,
		RecurrenceIDLocal: recurrenceIDLocal,
	})
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPosted looks up the ANNOUNCE message already posted for one occurrence,
// if any.
func (s *Store) GetPosted(ctx context.Context, automationID, uid, recurrenceIDLocal string) (PostedOccurrence, error) {
	row, err := s.q.GetPosted(ctx, queries.GetPostedParams{
		AutomationID:      automationID,
		Uid:               uid,
		RecurrenceIDLocal: recurrenceIDLocal,
	})
	if err != nil {
		return PostedOccurrence{}, mapNotFound(err)
	}
	return postedFrom(row), nil
}

// UpsertPosted records that an occurrence's announcement was posted or
// re-edited at sequence, and where. A second call for the same occurrence
// updates the row in place: an announcement is posted once and edited after
// that, never posted twice.
func (s *Store) UpsertPosted(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, channelID, messageID string) (PostedOccurrence, error) {
	var counts int4s
	params := queries.UpsertPostedParams{
		AutomationID:      automationID,
		Uid:               uid,
		RecurrenceIDLocal: recurrenceIDLocal,
		Sequence:          counts.of("sequence", sequence),
		ChannelID:         channelID,
		MessageID:         messageID,
	}
	if counts.err != nil {
		return PostedOccurrence{}, counts.err
	}
	row, err := s.q.UpsertPosted(ctx, params)
	if err != nil {
		return PostedOccurrence{}, mapConstraintError(mapNotFound(err))
	}
	return postedFrom(row), nil
}

// HasNudge reports whether a nudge row already exists for this occurrence,
// sequence, and trigger, so the scheduler never nudges the same trigger
// twice.
func (s *Store) HasNudge(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger string) (bool, error) {
	var counts int4s
	params := queries.HasNudgeParams{
		AutomationID:      automationID,
		Uid:               uid,
		RecurrenceIDLocal: recurrenceIDLocal,
		Sequence:          counts.of("sequence", sequence),
		Trigger:           trigger,
	}
	if counts.err != nil {
		return false, counts.err
	}
	return s.q.HasNudge(ctx, params)
}

// InsertNudge records one nudge outcome. The primary key on (automation_id,
// uid, recurrence_id_local, sequence, trigger) is what turns a second attempt
// at the same nudge into ErrConflict rather than a duplicate row.
func (s *Store) InsertNudge(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger, status string, messageID *string) (Nudge, error) {
	var counts int4s
	params := queries.InsertNudgeParams{
		AutomationID:      automationID,
		Uid:               uid,
		RecurrenceIDLocal: recurrenceIDLocal,
		Sequence:          counts.of("sequence", sequence),
		Trigger:           trigger,
		Status:            status,
		MessageID:         messageID,
	}
	if counts.err != nil {
		return Nudge{}, counts.err
	}
	row, err := s.q.InsertNudge(ctx, params)
	if err != nil {
		return Nudge{}, mapConstraintError(mapNotFound(err))
	}
	return Nudge{
		AutomationID:      row.AutomationID,
		UID:               row.Uid,
		RecurrenceIDLocal: row.RecurrenceIDLocal,
		Sequence:          int(row.Sequence),
		Trigger:           row.Trigger,
		Status:            row.Status,
		MessageID:         row.MessageID,
		CreatedAt:         row.CreatedAt,
	}, nil
}

func seenFrom(row queries.SeenOccurrence) SeenOccurrence {
	return SeenOccurrence{
		AutomationID:      row.AutomationID,
		UID:               row.Uid,
		RecurrenceIDLocal: row.RecurrenceIDLocal,
		Sequence:          int(row.Sequence),
		StartsAt:          row.StartsAt,
		EndsAt:            row.EndsAt,
		Title:             row.Title,
		ScopeID:           row.ScopeID,
		FirstSeenAt:       row.FirstSeenAt,
		LastSeenAt:        row.LastSeenAt,
	}
}

func postedFrom(row queries.PostedOccurrence) PostedOccurrence {
	return PostedOccurrence{
		AutomationID:      row.AutomationID,
		UID:               row.Uid,
		RecurrenceIDLocal: row.RecurrenceIDLocal,
		Sequence:          int(row.Sequence),
		ChannelID:         row.ChannelID,
		MessageID:         row.MessageID,
		PostedAt:          row.PostedAt,
	}
}

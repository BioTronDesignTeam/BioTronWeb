package store

import (
	"context"
	"time"
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
	rows, err := s.pool.Query(ctx, `
		SELECT `+automationColumns+`
		FROM automations WHERE enabled ORDER BY kind, lower(name)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	automations := []Automation{}
	for rows.Next() {
		automation, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		automations = append(automations, automation)
	}
	return automations, rows.Err()
}

// UpsertSeen records that automationID's fetch found occ just now.
// FirstSeenAt is set once, on the row's first insert; LastSeenAt moves on
// every call, which is what lets ListSeenFuture tell a still-fetched
// occurrence from one that stopped coming back.
func (s *Store) UpsertSeen(ctx context.Context, automationID string, occ SeenOccurrence) (SeenOccurrence, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO seen_occurrences (
			automation_id, uid, recurrence_id_local, sequence, starts_at,
			ends_at, title, scope_id, first_seen_at, last_seen_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::uuid, now(), now())
		ON CONFLICT (automation_id, uid, recurrence_id_local) DO UPDATE SET
			sequence = EXCLUDED.sequence,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			title = EXCLUDED.title,
			scope_id = EXCLUDED.scope_id,
			last_seen_at = now()
		RETURNING automation_id::text, uid, recurrence_id_local, sequence,
		          starts_at, ends_at, title, scope_id::text, first_seen_at, last_seen_at
	`, automationID, occ.UID, occ.RecurrenceIDLocal, occ.Sequence, occ.StartsAt,
		occ.EndsAt, occ.Title, occ.ScopeID)
	saved, err := scanSeenOccurrence(row)
	if err != nil {
		return SeenOccurrence{}, mapConstraintError(err)
	}
	return saved, nil
}

// ListSeenFuture returns every occurrence automationID last saw whose
// starts_at is still ahead of now. Anything already past is irrelevant to
// cancellation detection: an event Calendar stops returning because it has
// already happened is not a cancellation.
func (s *Store) ListSeenFuture(ctx context.Context, automationID string, now time.Time) ([]SeenOccurrence, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT automation_id::text, uid, recurrence_id_local, sequence,
		       starts_at, ends_at, title, scope_id::text, first_seen_at, last_seen_at
		FROM seen_occurrences
		WHERE automation_id = $1::uuid AND starts_at > $2
		ORDER BY starts_at
	`, automationID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := []SeenOccurrence{}
	for rows.Next() {
		occ, err := scanSeenOccurrence(rows)
		if err != nil {
			return nil, err
		}
		seen = append(seen, occ)
	}
	return seen, rows.Err()
}

// DeleteSeen forgets one occurrence, once the scheduler has finished
// reacting to its cancellation.
func (s *Store) DeleteSeen(ctx context.Context, automationID, uid, recurrenceIDLocal string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM seen_occurrences
		WHERE automation_id = $1::uuid AND uid = $2 AND recurrence_id_local = $3
	`, automationID, uid, recurrenceIDLocal)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPosted looks up the ANNOUNCE message already posted for one occurrence,
// if any.
func (s *Store) GetPosted(ctx context.Context, automationID, uid, recurrenceIDLocal string) (PostedOccurrence, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT automation_id::text, uid, recurrence_id_local, sequence,
		       channel_id, message_id, posted_at
		FROM posted_occurrences
		WHERE automation_id = $1::uuid AND uid = $2 AND recurrence_id_local = $3
	`, automationID, uid, recurrenceIDLocal)
	return scanPostedOccurrence(row)
}

// UpsertPosted records that an occurrence's announcement was posted or
// re-edited at sequence, and where. A second call for the same occurrence
// updates the row in place: an announcement is posted once and edited after
// that, never posted twice.
func (s *Store) UpsertPosted(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, channelID, messageID string) (PostedOccurrence, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO posted_occurrences (
			automation_id, uid, recurrence_id_local, sequence, channel_id,
			message_id, posted_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, now())
		ON CONFLICT (automation_id, uid, recurrence_id_local) DO UPDATE SET
			sequence = EXCLUDED.sequence,
			channel_id = EXCLUDED.channel_id,
			message_id = EXCLUDED.message_id,
			posted_at = now()
		RETURNING automation_id::text, uid, recurrence_id_local, sequence,
		          channel_id, message_id, posted_at
	`, automationID, uid, recurrenceIDLocal, sequence, channelID, messageID)
	saved, err := scanPostedOccurrence(row)
	if err != nil {
		return PostedOccurrence{}, mapConstraintError(err)
	}
	return saved, nil
}

// HasNudge reports whether a nudge row already exists for this occurrence,
// sequence, and trigger, so the scheduler never nudges the same trigger
// twice.
func (s *Store) HasNudge(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM nudges
			WHERE automation_id = $1::uuid AND uid = $2 AND recurrence_id_local = $3
			  AND sequence = $4 AND trigger = $5::"NudgeTrigger"
		)
	`, automationID, uid, recurrenceIDLocal, sequence, trigger).Scan(&exists)
	return exists, err
}

// InsertNudge records one nudge outcome. The primary key on (automation_id,
// uid, recurrence_id_local, sequence, trigger) is what turns a second attempt
// at the same nudge into ErrConflict rather than a duplicate row.
func (s *Store) InsertNudge(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger, status string, messageID *string) (Nudge, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO nudges (
			automation_id, uid, recurrence_id_local, sequence, trigger,
			status, message_id
		) VALUES ($1::uuid, $2, $3, $4, $5::"NudgeTrigger", $6::"NudgeStatus", $7)
		RETURNING automation_id::text, uid, recurrence_id_local, sequence,
		          trigger::text, status::text, message_id, created_at
	`, automationID, uid, recurrenceIDLocal, sequence, trigger, status, messageID)
	saved, err := scanNudge(row)
	if err != nil {
		return Nudge{}, mapConstraintError(err)
	}
	return saved, nil
}

func scanSeenOccurrence(row scanner) (SeenOccurrence, error) {
	var occ SeenOccurrence
	if err := row.Scan(&occ.AutomationID, &occ.UID, &occ.RecurrenceIDLocal,
		&occ.Sequence, &occ.StartsAt, &occ.EndsAt, &occ.Title, &occ.ScopeID,
		&occ.FirstSeenAt, &occ.LastSeenAt); err != nil {
		return SeenOccurrence{}, mapNotFound(err)
	}
	return occ, nil
}

func scanPostedOccurrence(row scanner) (PostedOccurrence, error) {
	var occ PostedOccurrence
	if err := row.Scan(&occ.AutomationID, &occ.UID, &occ.RecurrenceIDLocal,
		&occ.Sequence, &occ.ChannelID, &occ.MessageID, &occ.PostedAt); err != nil {
		return PostedOccurrence{}, mapNotFound(err)
	}
	return occ, nil
}

func scanNudge(row scanner) (Nudge, error) {
	var nudge Nudge
	if err := row.Scan(&nudge.AutomationID, &nudge.UID, &nudge.RecurrenceIDLocal,
		&nudge.Sequence, &nudge.Trigger, &nudge.Status, &nudge.MessageID,
		&nudge.CreatedAt); err != nil {
		return Nudge{}, mapNotFound(err)
	}
	return nudge, nil
}

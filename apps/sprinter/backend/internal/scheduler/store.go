package scheduler

import (
	"context"
	"time"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// Store is the part of the database the scheduler uses, kept small and owned
// by this package so a tick can be tested against an in-memory fake rather
// than a live database.
type Store interface {
	ListEnabledAutomations(ctx context.Context) ([]store.Automation, error)
	UpsertSeen(ctx context.Context, automationID string, occ store.SeenOccurrence) (store.SeenOccurrence, error)
	ListSeenFuture(ctx context.Context, automationID string, now time.Time) ([]store.SeenOccurrence, error)
	DeleteSeen(ctx context.Context, automationID, uid, recurrenceIDLocal string) error
	GetPosted(ctx context.Context, automationID, uid, recurrenceIDLocal string) (store.PostedOccurrence, error)
	UpsertPosted(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, channelID, messageID string) (store.PostedOccurrence, error)
	HasNudge(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger string) (bool, error)
	InsertNudge(ctx context.Context, automationID, uid, recurrenceIDLocal string, sequence int, trigger, status string, messageID *string) (store.Nudge, error)
}

// Calendar is the part of Calendar's API the scheduler reads.
type Calendar interface {
	Occurrences(ctx context.Context, scopeID string, from, to time.Time) ([]calendar.Occurrence, error)
}

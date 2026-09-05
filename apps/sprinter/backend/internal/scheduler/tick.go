package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// occurrenceKey identifies one occurrence within an automation's scope,
// independent of its sequence: a series edit changes the sequence but not
// the uid or the occurrence's original local start.
type occurrenceKey struct {
	uid               string
	recurrenceIDLocal string
}

// fetchWindow is how far ahead every tick asks Calendar, whatever an
// automation's lead_hours says. It is the largest lead an automation may
// carry — 168 hours, checked in the admin API — plus an hour of slack.
//
// The window is fixed on purpose. Cancellation is inferred by comparing what
// the automation last saw against what Calendar now returns, so the two have
// to cover the same span. A window that moved with lead_hours would read
// "somebody lowered lead_hours" as "everything was cancelled", and so would
// a scope_id edit or an occurrence drifting past the window's edge.
const fetchWindow = 169 * time.Hour

// Tick is the unit of work: one pass over every enabled automation. Each
// automation's failure is caught here so that one broken automation — a bad
// scope id, a Discord channel the bot was removed from — can never stop the
// others from running.
func (s *Scheduler) Tick(ctx context.Context) error {
	automations, err := s.store.ListEnabledAutomations(ctx)
	if err != nil {
		return fmt.Errorf("list enabled automations: %w", err)
	}
	for _, automation := range automations {
		s.runAutomation(ctx, automation)
	}
	return nil
}

func (s *Scheduler) runAutomation(ctx context.Context, automation store.Automation) {
	defer func() {
		if r := recover(); r != nil {
			s.emit(logclient.Error, "Automation failed", map[string]any{
				"automation_id": automation.ID, "error": fmt.Sprintf("panic: %v", r),
			})
		}
	}()
	if err := s.processAutomation(ctx, automation); err != nil {
		s.emit(logclient.Error, "Automation failed", map[string]any{
			"automation_id": automation.ID, "error": err.Error(),
		})
	}
}

func (s *Scheduler) processAutomation(ctx context.Context, automation store.Automation) error {
	now := s.now()
	// One fetch over the fixed window serves both jobs. Announcing and
	// nudging then filter it by lead_hours in Go, which they already do.
	windowEnd := now.Add(fetchWindow)
	occurrences, err := s.calendar.Occurrences(ctx, automation.ScopeID, now, windowEnd)
	if err != nil {
		// A fetch failure means "unknown", never "empty": skip this
		// automation entirely rather than treat a Calendar outage as
		// everything on it having been cancelled.
		s.emit(logclient.Error, "Calendar fetch failed", map[string]any{
			"automation_id": automation.ID, "error": err.Error(),
		})
		return nil
	}

	fetched := make(map[occurrenceKey]calendar.Occurrence, len(occurrences))
	for _, occ := range occurrences {
		fetched[occurrenceKey{uid: occ.UID, recurrenceIDLocal: occ.RecurrenceID}] = occ
	}

	if err := s.detectCancellations(ctx, automation, now, windowEnd, fetched); err != nil {
		return err
	}

	for _, occ := range occurrences {
		seen := store.SeenOccurrence{
			UID: occ.UID, RecurrenceIDLocal: occ.RecurrenceID, Sequence: occ.Sequence(),
			StartsAt: occ.StartsAt, EndsAt: occ.EndsAt, Title: occ.Title, ScopeID: occ.ScopeID,
		}
		if _, err := s.store.UpsertSeen(ctx, automation.ID, seen); err != nil {
			return fmt.Errorf("upsert seen occurrence %s: %w", occ.UID, err)
		}
	}

	switch automation.Kind {
	case store.KindAnnounce:
		return s.processAnnounce(ctx, automation, occurrences, now)
	case store.KindNudge:
		return s.processNudge(ctx, automation, occurrences, now)
	default:
		return fmt.Errorf("automation kind %q is not ANNOUNCE or NUDGE", automation.Kind)
	}
}

// detectCancellations finds every occurrence this automation last saw with a
// still-future start that the fresh fetch no longer contains. Calendar's
// public JSON drops a cancelled occurrence rather than flagging it, so
// "expected to still be coming, and is not" is the only signal there is.
//
// windowEnd is where the fetch stopped. A seen occurrence starting past it
// was never in this fetch to begin with, so its absence says nothing about
// whether it still exists, and it is left alone.
func (s *Scheduler) detectCancellations(ctx context.Context, automation store.Automation, now, windowEnd time.Time, fetched map[occurrenceKey]calendar.Occurrence) error {
	seenFuture, err := s.store.ListSeenFuture(ctx, automation.ID, now)
	if err != nil {
		return fmt.Errorf("list seen future occurrences: %w", err)
	}
	for _, seen := range seenFuture {
		if _, ok := fetched[occurrenceKey{uid: seen.UID, recurrenceIDLocal: seen.RecurrenceIDLocal}]; ok {
			continue
		}
		if seen.StartsAt.After(windowEnd) {
			continue
		}
		s.emit(logclient.Info, "Occurrence vanished", map[string]any{
			"automation_id": automation.ID, "uid": seen.UID,
			"recurrence_id_local": seen.RecurrenceIDLocal,
		})
		switch automation.Kind {
		case store.KindNudge:
			if err := s.runNudge(ctx, automation, seen.UID, seen.RecurrenceIDLocal, seen.Sequence,
				store.TriggerCancelled, seen.Title, seen.StartsAt); err != nil {
				return err
			}
		case store.KindAnnounce:
			if err := s.editCancelledAnnouncement(ctx, automation, seen); err != nil {
				return err
			}
		}
		if err := s.store.DeleteSeen(ctx, automation.ID, seen.UID, seen.RecurrenceIDLocal); err != nil &&
			!errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("delete seen occurrence %s: %w", seen.UID, err)
		}
	}
	return nil
}

func (s *Scheduler) editCancelledAnnouncement(ctx context.Context, automation store.Automation, seen store.SeenOccurrence) error {
	posted, err := s.store.GetPosted(ctx, automation.ID, seen.UID, seen.RecurrenceIDLocal)
	if errors.Is(err, store.ErrNotFound) {
		// Nothing was ever posted for it, so there is nothing to edit.
		return nil
	}
	if err != nil {
		return fmt.Errorf("get posted occurrence %s: %w", seen.UID, err)
	}
	content := "Cancelled: " + seen.Title + "\n" +
		"This was scheduled for " + seen.StartsAt.In(s.location).Format(announceTimeLayout) + " and has been cancelled."
	if err := s.discord.EditMessage(posted.ChannelID, posted.MessageID, content); err != nil {
		return fmt.Errorf("edit cancelled announcement %s: %w", posted.MessageID, err)
	}
	return nil
}

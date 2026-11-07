package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
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
// The window is fixed on purpose. A vanished occurrence is noticed by
// comparing what the automation last saw against what Calendar now returns,
// so the two have to cover the same span. A window that moved with
// lead_hours would make every lowering of lead_hours look like a week of
// vanished occurrences, each one costing a feed lookup.
const fetchWindow = 169 * time.Hour

// errCalendarFetch ends one automation's pass early without failing it. The
// fetch failure is reported where it happens, and the next tick tries again.
var errCalendarFetch = errors.New("calendar fetch failed")

// Tick is the unit of work: one pass over every enabled automation. Each
// automation's failure is caught here so that one broken automation — a bad
// scope id, a Discord channel the bot was removed from — can never stop the
// others from running.
func (s *Scheduler) Tick(ctx context.Context) error {
	automations, err := s.store.ListEnabledAutomations(ctx)
	if err != nil {
		return fmt.Errorf("list enabled automations: %w", err)
	}
	// One feed read serves every automation in this tick, and only a tick
	// in which something vanished pays for it.
	feed := &feedLookup{calendar: s.calendar}
	for _, automation := range automations {
		s.runAutomation(ctx, automation, feed)
	}
	return nil
}

func (s *Scheduler) runAutomation(ctx context.Context, automation store.Automation, feed *feedLookup) {
	defer func() {
		if r := recover(); r != nil {
			s.emit(logclient.Error, "Automation failed", map[string]any{
				"automation_id": automation.ID, "error": fmt.Sprintf("panic: %v", r),
			})
		}
	}()
	if err := s.processAutomation(ctx, automation, feed); err != nil && !errors.Is(err, errCalendarFetch) {
		s.emit(logclient.Error, "Automation failed", map[string]any{
			"automation_id": automation.ID, "error": err.Error(),
		})
	}
}

func (s *Scheduler) processAutomation(ctx context.Context, automation store.Automation, feed *feedLookup) error {
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
		return errCalendarFetch
	}

	fetched := make(map[occurrenceKey]calendar.Occurrence, len(occurrences))
	for _, occ := range occurrences {
		fetched[occurrenceKey{uid: occ.UID, recurrenceIDLocal: occ.RecurrenceID}] = occ
	}

	if err := s.detectCancellations(ctx, automation, now, windowEnd, fetched, feed); err != nil {
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
// still-future start that the fresh fetch no longer contains, and asks
// Calendar's feed why.
//
// Vanishing from /v1/events is not a cancellation by itself. That view
// drops a cancelled occurrence, but it also drops one that moved past the
// window's end, one whose scope (or an ancestor) was archived, and one that
// moved to another scope. Only an occurrence the feed marks cancelled, or
// no longer has at all, is announced as cancelled. Any other is forgotten
// quietly: its announcement stays as it was, and if it comes back into view
// it is seen again, and its post edited when its sequence has risen.
//
// windowEnd is where the fetch stopped. A seen occurrence starting past it
// was never in this fetch to begin with, so its absence says nothing about
// whether it still exists, and it is left alone.
func (s *Scheduler) detectCancellations(ctx context.Context, automation store.Automation, now, windowEnd time.Time, fetched map[occurrenceKey]calendar.Occurrence, feed *feedLookup) error {
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
		index, err := feed.get(ctx)
		if err != nil {
			// The feed is the only thing that can say "cancelled". Without
			// it nothing is decided, and the seen rows wait for a tick that
			// can read it.
			s.emit(logclient.Error, "Calendar fetch failed", map[string]any{
				"automation_id": automation.ID, "error": err.Error(), "source": "feed",
			})
			return errCalendarFetch
		}
		status := index.Status(seen.UID, seen.RecurrenceIDLocal)
		s.emit(logclient.Info, "Occurrence vanished", map[string]any{
			"automation_id": automation.ID, "uid": seen.UID,
			"recurrence_id_local": seen.RecurrenceIDLocal, "status": status.String(),
		})
		if status != calendar.StatusExists {
			if err := s.announceCancellation(ctx, automation, seen); err != nil {
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

func (s *Scheduler) announceCancellation(ctx context.Context, automation store.Automation, seen store.SeenOccurrence) error {
	switch automation.Kind {
	case store.KindNudge:
		return s.runNudge(ctx, automation, seen.UID, seen.RecurrenceIDLocal, seen.Sequence,
			store.TriggerCancelled, seen.Title, seen.StartsAt)
	case store.KindAnnounce:
		return s.editCancelledAnnouncement(ctx, automation, seen)
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

// feedLookup reads Calendar's whole feed at most once per tick, the first
// time an automation needs to know why an occurrence vanished. A failed read
// is remembered for the rest of the tick too, so one Calendar outage is one
// failed request, not one per automation.
type feedLookup struct {
	calendar Calendar
	feed     *calendar.Feed
	err      error
	done     bool
}

func (l *feedLookup) get(ctx context.Context) (*calendar.Feed, error) {
	if !l.done {
		l.feed, l.err = l.calendar.Feed(ctx)
		l.done = true
	}
	return l.feed, l.err
}

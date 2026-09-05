package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// announceTimeLayout is how an announcement, an edit, or a cancellation note
// shows an occurrence's local time.
const announceTimeLayout = "Monday, January 2 at 3:04 PM MST"

// processAnnounce posts or edits one message per occurrence entering the
// lead window. The message is a fixed template, never a model call: an
// announcement states facts Calendar already confirmed, so there is nothing
// for a model to add.
func (s *Scheduler) processAnnounce(ctx context.Context, automation store.Automation, occurrences []calendar.Occurrence, now time.Time) error {
	lead := time.Duration(automation.LeadHours) * time.Hour
	for _, occ := range occurrences {
		if occ.StartsAt.Sub(now) > lead {
			continue
		}
		// post_hour holds an announcement back until the wall clock, not the
		// occurrence's own start, reaches that local hour — so a tick that
		// fires at 3 AM does not post to the channel in the middle of the
		// night.
		if automation.PostHour != nil && now.In(s.location).Hour() < *automation.PostHour {
			continue
		}
		if err := s.postOrEditAnnouncement(ctx, automation, occ); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) postOrEditAnnouncement(ctx context.Context, automation store.Automation, occ calendar.Occurrence) error {
	sequence := occ.Sequence()
	content := announceContent(occ, s.location, false)

	posted, err := s.store.GetPosted(ctx, automation.ID, occ.UID, occ.RecurrenceID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// An announcement pings nobody: its text is an event title and a
		// location, both written by whoever put the event on the calendar.
		messageID, sendErr := s.discord.SendMessage(automation.ChannelID, content, nil)
		if sendErr != nil {
			return fmt.Errorf("post announcement for %s: %w", occ.UID, sendErr)
		}
		if _, err := s.store.UpsertPosted(ctx, automation.ID, occ.UID, occ.RecurrenceID, sequence, automation.ChannelID, messageID); err != nil {
			return fmt.Errorf("record posted occurrence %s: %w", occ.UID, err)
		}
		s.emit(logclient.Info, "Announcement posted", map[string]any{
			"automation_id": automation.ID, "uid": occ.UID, "message_id": messageID,
		})
		return nil
	case err != nil:
		return fmt.Errorf("get posted occurrence %s: %w", occ.UID, err)
	case posted.Sequence < sequence:
		if err := s.discord.EditMessage(automation.ChannelID, posted.MessageID, content); err != nil {
			return fmt.Errorf("edit announcement %s: %w", posted.MessageID, err)
		}
		if _, err := s.store.UpsertPosted(ctx, automation.ID, occ.UID, occ.RecurrenceID, sequence, automation.ChannelID, posted.MessageID); err != nil {
			return fmt.Errorf("record updated occurrence %s: %w", occ.UID, err)
		}
		s.emit(logclient.Info, "Announcement updated", map[string]any{
			"automation_id": automation.ID, "uid": occ.UID, "message_id": posted.MessageID,
		})
		return nil
	default:
		// Already posted at this sequence or later: nothing to do.
		return nil
	}
}

func announceContent(occ calendar.Occurrence, location *time.Location, cancelled bool) string {
	var b strings.Builder
	if cancelled {
		b.WriteString("Cancelled: ")
	}
	b.WriteString(occ.Title)
	b.WriteString("\n")
	b.WriteString(occ.StartsAt.In(location).Format(announceTimeLayout))
	if occ.Location != "" {
		b.WriteString(" · ")
		b.WriteString(occ.Location)
	}
	if occ.URL != "" {
		b.WriteString("\n")
		b.WriteString(occ.URL)
	}
	return b.String()
}

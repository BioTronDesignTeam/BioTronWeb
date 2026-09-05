package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// nudgeTimeLayout is the shorter local-time format a nudge uses, since it
// speaks to the lead rather than posting to the whole channel.
const nudgeTimeLayout = "Mon Jan 2, 3:04 PM"

// channelHistoryPageLimit bounds one ChannelMessages call. channelSatisfied
// pages through at most channelHistoryMaxPages of these before giving up, so
// an unusually busy channel cannot turn one tick into an unbounded scan.
const (
	channelHistoryPageLimit = 100
	channelHistoryMaxPages  = 20
)

// processNudge finds every occurrence entering the lead window that has not
// already been nudged at its current sequence, and runs the nudge flow for
// each.
func (s *Scheduler) processNudge(ctx context.Context, automation store.Automation, occurrences []calendar.Occurrence, now time.Time) error {
	if automation.LeadUserID == nil {
		return errors.New("nudge automation has no lead_user_id")
	}
	lead := time.Duration(automation.LeadHours) * time.Hour
	for _, occ := range occurrences {
		if occ.StartsAt.Sub(now) > lead {
			continue
		}
		sequence := occ.Sequence()
		has, err := s.store.HasNudge(ctx, automation.ID, occ.UID, occ.RecurrenceID, sequence, store.TriggerUpcoming)
		if err != nil {
			return fmt.Errorf("check nudge %s: %w", occ.UID, err)
		}
		if has {
			continue
		}
		if err := s.runNudge(ctx, automation, occ.UID, occ.RecurrenceID, sequence,
			store.TriggerUpcoming, occ.Title, occ.StartsAt); err != nil {
			return err
		}
	}
	return nil
}

// runNudge is the nudge flow shared by an occurrence entering the lead
// window (trigger UPCOMING) and one that vanished from Calendar while still
// in the future (trigger CANCELLED). It checks whether the lead already
// announced the occurrence themselves, and if not, sends them a nudge with a
// drafted announcement.
func (s *Scheduler) runNudge(ctx context.Context, automation store.Automation, uid, recurrenceIDLocal string, sequence int, trigger, title string, startsAt time.Time) error {
	if automation.LeadUserID == nil {
		return errors.New("nudge automation has no lead_user_id")
	}
	// The channel scan asks "has the lead already announced this?", which
	// only answers an UPCOMING nudge. A cancellation is the opposite case:
	// the announcement inside the lookback window is the very message that
	// now needs correcting, so letting it satisfy the nudge would silence
	// the one nudge nobody else can send.
	if trigger != store.TriggerCancelled {
		since := s.now().Add(-time.Duration(automation.LookbackHours) * time.Hour)
		satisfied, err := s.channelSatisfied(automation, since)
		if err != nil {
			return fmt.Errorf("scan announcement channel: %w", err)
		}
		if satisfied {
			if _, err := s.store.InsertNudge(ctx, automation.ID, uid, recurrenceIDLocal, sequence, trigger, store.NudgeSatisfied, nil); err != nil &&
				!errors.Is(err, store.ErrConflict) {
				return fmt.Errorf("record satisfied nudge %s: %w", uid, err)
			}
			s.emit(logclient.Info, "Nudge satisfied", map[string]any{
				"automation_id": automation.ID, "uid": uid, "trigger": trigger,
			})
			return nil
		}
	}

	draft, draftErr := s.draftAnnouncement(ctx, title, startsAt, trigger)
	body := nudgeBody(s.location, title, startsAt, draft, draftErr)

	var (
		messageID string
		sendErr   error
		delivered string
	)
	if automation.Deliver == store.DeliverChannel {
		delivered = "channel"
		// The lead is the one id this message may ping. The rest of the body
		// carries an event title and a model's draft, and neither is allowed
		// to reach @everyone.
		messageID, sendErr = s.discord.SendMessage(automation.ChannelID,
			"<@"+*automation.LeadUserID+"> "+body, []string{*automation.LeadUserID})
	} else {
		delivered = "dm"
		messageID, sendErr = s.discord.DirectMessage(*automation.LeadUserID, body)
	}
	if sendErr != nil {
		return fmt.Errorf("send nudge %s: %w", uid, sendErr)
	}

	sentMessageID := messageID
	if _, err := s.store.InsertNudge(ctx, automation.ID, uid, recurrenceIDLocal, sequence, trigger, store.NudgeSent, &sentMessageID); err != nil &&
		!errors.Is(err, store.ErrConflict) {
		return fmt.Errorf("record sent nudge %s: %w", uid, err)
	}
	s.emit(logclient.Info, "Nudge sent", map[string]any{
		"automation_id": automation.ID, "uid": uid, "trigger": trigger, "delivered": delivered,
	})
	return nil
}

// channelSatisfied reports whether the announcement channel already carries
// a message, sent since since, from the lead (or from anyone, when the
// automation allows any author).
//
// Discord's before, after, and around parameters are mutually exclusive, so
// the walk uses after alone. A query with after returns the messages just
// after that id, the oldest end of the window, so each further page moves
// afterID up to the newest id seen. The walk stops when a page comes back
// short, which means the window is exhausted.
func (s *Scheduler) channelSatisfied(automation store.Automation, since time.Time) (bool, error) {
	afterID := snowflakeAfter(since)
	for page := 0; page < channelHistoryMaxPages; page++ {
		messages, err := s.discord.ChannelMessages(automation.ChannelID, channelHistoryPageLimit, "", afterID)
		if err != nil {
			return false, err
		}
		if len(messages) == 0 {
			return false, nil
		}
		newestID := messages[0].ID
		for _, message := range messages {
			if snowflakeLess(newestID, message.ID) {
				newestID = message.ID
			}
			if message.Timestamp.Before(since) {
				continue
			}
			if automation.AnyAuthor && !message.IsBot {
				return true, nil
			}
			if automation.LeadUserID != nil && message.AuthorID == *automation.LeadUserID {
				return true, nil
			}
		}
		if len(messages) < channelHistoryPageLimit {
			return false, nil
		}
		afterID = newestID
	}
	return false, nil
}

// snowflakeLess compares two Discord snowflake ids as the numbers they are,
// not lexicographically: two ids from the same short window are almost
// always the same digit length, but a plain string compare would get the
// order wrong the moment they are not.
func snowflakeLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// draftAnnouncement asks the model for a short announcement the lead can use
// as-is. A model error is never fatal to the nudge itself: nudgeBody says the
// draft failed and the nudge still goes out.
func (s *Scheduler) draftAnnouncement(ctx context.Context, title string, startsAt time.Time, trigger string) (string, error) {
	if s.model == nil {
		return "", errors.New("no model configured")
	}
	when := startsAt.In(s.location).Format(announceTimeLayout)
	var ask string
	if trigger == store.TriggerCancelled {
		ask = fmt.Sprintf("Write a short Discord announcement telling the team that %q, previously scheduled for %s, has been cancelled. This is a cancellation notice.", title, when)
	} else {
		ask = fmt.Sprintf("Write a short Discord announcement for %q, scheduled for %s.", title, when)
	}
	response, err := s.model.Generate(ctx,
		"You write short Discord announcements for a university robotics team.",
		[]model.Message{{Role: model.RoleUser, Text: ask}}, nil)
	if err != nil {
		return "", err
	}
	return response.Text, nil
}

// nudgeBody is the message the lead sees, whether by DM or by a channel
// mention. It never goes to the announcement channel itself: that channel is
// only ever posted to by the ANNOUNCE flow or by the lead's own message.
func nudgeBody(location *time.Location, title string, startsAt time.Time, draft string, draftErr error) string {
	when := startsAt.In(location).Format(nudgeTimeLayout)
	base := fmt.Sprintf("No announcement has gone out for %q on %s yet.", title, when)
	if draftErr != nil {
		return base + " A draft could not be generated; please write your own."
	}
	return base + " Here is a suggested draft:\n\n" + draft
}

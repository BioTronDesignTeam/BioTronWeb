// Package scheduler is Sprinter's clock: it polls Calendar on an interval
// and turns what it finds into two things — ANNOUNCE automations post and
// edit Calendar events in a channel, NUDGE automations chase a lead who has
// not announced their own event yet.
//
// Calendar's public JSON drops a cancelled occurrence instead of flagging
// it, so the scheduler keeps its own memory of what it last saw
// (store.SeenOccurrence). A future occurrence that stopped coming back is
// looked up in Calendar's feed, and only one the feed marks cancelled or no
// longer has is treated as cancelled. Everything else it needs — Store,
// Calendar, Discord, and the model that drafts a nudge's suggested
// announcement — is a small interface this package owns, so a tick can be
// tested with in-memory fakes and a fixed clock.
package scheduler

import (
	"context"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// DefaultModelTimeout bounds one nudge draft. A draft is a few sentences; a
// model that has not written them in a minute is not going to, and the nudge
// goes out without one.
const DefaultModelTimeout = time.Minute

// Deps is everything a Scheduler needs. Every field is required except
// Location, Now and ModelTimeout, which default to UTC, time.Now and
// DefaultModelTimeout.
type Deps struct {
	Store    Store
	Calendar Calendar
	Discord  DiscordAPI
	Model    model.Model
	Events   *logclient.Client
	// Location is the timezone every "local hour" and "local time" decision
	// is made in — America/Toronto in production, the one Calendar itself
	// runs in.
	Location *time.Location
	// Now is the clock. Tests pin it; production leaves it nil for time.Now.
	Now func() time.Time
	// ModelTimeout bounds one nudge draft. Zero means DefaultModelTimeout.
	ModelTimeout time.Duration
}

type Scheduler struct {
	store        Store
	calendar     Calendar
	discord      DiscordAPI
	model        model.Model
	events       *logclient.Client
	location     *time.Location
	now          func() time.Time
	modelTimeout time.Duration
}

func New(deps Deps) *Scheduler {
	location := deps.Location
	if location == nil {
		location = time.UTC
	}
	modelTimeout := deps.ModelTimeout
	if modelTimeout <= 0 {
		modelTimeout = DefaultModelTimeout
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Scheduler{
		store:        deps.Store,
		calendar:     deps.Calendar,
		discord:      deps.Discord,
		model:        deps.Model,
		events:       deps.Events,
		location:     location,
		now:          now,
		modelTimeout: modelTimeout,
	}
}

// Run ticks every interval until ctx is done, running one Tick immediately
// so the first automation does not wait a full interval after start.
//
// Cancelling ctx stops the scheduler between messages, never between a
// message and its record: a tick sends nothing once ctx is done, and a
// message already sent is still written down (recordContext). Run returns
// once the tick in flight has, so the caller can wait for it before closing
// the store and the Discord session. Otherwise a message posted just before
// shutdown would go unrecorded and be posted again after the restart.
func (s *Scheduler) Run(ctx context.Context, every time.Duration) {
	s.tickAndLog(ctx)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tickAndLog(ctx)
		}
	}
}

// recordTimeout bounds the write that records a message already sent.
const recordTimeout = 10 * time.Second

// recordContext is for the write that follows a message Discord has already
// accepted. It outlives ctx's cancellation, so a shutdown arriving between
// the send and the write cannot leave a sent message unrecorded, which the
// next run would send again.
func recordContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
}

func (s *Scheduler) tickAndLog(ctx context.Context) {
	if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
		s.emit(logclient.Error, "Scheduler tick failed", map[string]any{"error": err.Error()})
	}
}

func (s *Scheduler) emit(level logclient.Level, message string, payload any) {
	if s.events == nil {
		return
	}
	s.events.LogAsync(level, message, payload)
}

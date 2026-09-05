// Package scheduler is Sprinter's clock: it polls Calendar on an interval
// and turns what it finds into two things — ANNOUNCE automations post and
// edit Calendar events in a channel, NUDGE automations chase a lead who has
// not announced their own event yet.
//
// Calendar's public JSON drops a cancelled occurrence instead of flagging
// it, so the scheduler keeps its own memory of what it last saw
// (store.SeenOccurrence) and treats a future occurrence that stopped coming
// back as cancelled. Everything else it needs — Store, Calendar, Discord,
// and the model that drafts a nudge's suggested announcement — is a small
// interface this package owns, so a tick can be tested with in-memory fakes
// and a fixed clock.
package scheduler

import (
	"context"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

// Deps is everything a Scheduler needs. Every field is required except
// Location and Now, which default to UTC and time.Now.
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
}

type Scheduler struct {
	store    Store
	calendar Calendar
	discord  DiscordAPI
	model    model.Model
	events   *logclient.Client
	location *time.Location
	now      func() time.Time
}

func New(deps Deps) *Scheduler {
	location := deps.Location
	if location == nil {
		location = time.UTC
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Scheduler{
		store:    deps.Store,
		calendar: deps.Calendar,
		discord:  deps.Discord,
		model:    deps.Model,
		events:   deps.Events,
		location: location,
		now:      now,
	}
}

// Run ticks every interval until ctx is done, running one Tick immediately
// so the first automation does not wait a full interval after start.
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

func (s *Scheduler) tickAndLog(ctx context.Context) {
	if err := s.Tick(ctx); err != nil {
		s.emit(logclient.Error, "Scheduler tick failed", map[string]any{"error": err.Error()})
	}
}

func (s *Scheduler) emit(level logclient.Level, message string, payload any) {
	if s.events == nil {
		return
	}
	s.events.LogAsync(level, message, payload)
}

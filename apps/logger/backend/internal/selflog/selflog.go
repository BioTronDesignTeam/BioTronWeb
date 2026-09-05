// Package selflog records Logger's own events. Every other service posts to
// Logger's ingest route, but Logger cannot: each event would arrive as a
// request, and the request log would record it, so the service would log its
// own logging forever. The recorder writes straight into the store instead.
package selflog

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

// service is the catalog id Logger reports under. It matches the logger-api
// component in the catalog, so its own events sit beside its health.
const service = "logger-api"

// Store is the part of the data store the recorder needs.
type Store interface {
	InsertLog(context.Context, model.NewLog) (model.Log, error)
}

// Recorder writes Logger's own events into the store. It satisfies
// fiberlog.Sink, so the request log and the events Logger raises itself take
// one path.
type Recorder struct {
	store Store
	slots chan struct{}
}

// New returns a recorder that holds at most 32 events in flight, the ceiling
// logclient gives every other service.
func New(store Store) *Recorder {
	return &Recorder{store: store, slots: make(chan struct{}, 32)}
}

// LogAsync records one event in the background with a two-second timeout. Past
// 32 events in flight it drops the event and says so on stdout, so a slow
// store cannot stall a request or the health poll.
func (r *Recorder) LogAsync(level logclient.Level, message string, payload any) {
	if r == nil {
		return
	}
	select {
	case r.slots <- struct{}{}:
	default:
		log.Print("Logger event delivery busy; dropping event")
		return
	}
	go func() {
		defer func() { <-r.slots }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		r.Log(ctx, level, message, payload)
	}()
}

// Log records one event and waits for the store. Shutdown uses it: the process
// is about to exit and a background goroutine would go with it.
func (r *Recorder) Log(ctx context.Context, level logclient.Level, message string, payload any) {
	if r == nil {
		return
	}
	var raw json.RawMessage
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			log.Printf("marshal Logger event: %v", err)
			return
		}
	}
	if _, err := r.store.InsertLog(ctx, model.NewLog{
		Service: service,
		Level:   storeLevel(level),
		Message: message,
		Payload: raw,
	}); err != nil {
		// Stdout is the only place left. An event reporting this failure would
		// go to the store that just refused one.
		log.Printf("store Logger event: %v", err)
	}
}

// storeLevel maps the client's levels onto the store's. The two spell the same
// four words as different types. Anything else becomes info, so a miswritten
// call still reaches the explorer instead of vanishing.
func storeLevel(level logclient.Level) model.LogLevel {
	switch level {
	case logclient.Debug:
		return model.LevelDebug
	case logclient.Warning:
		return model.LevelWarning
	case logclient.Error:
		return model.LevelError
	default:
		return model.LevelInfo
	}
}

// Discard drops every event. Callers that hold no store use it instead of a
// nil interface, which would panic on the first event.
type Discard struct{}

func (Discard) LogAsync(logclient.Level, string, any) {}

func (Discard) Log(context.Context, logclient.Level, string, any) {}

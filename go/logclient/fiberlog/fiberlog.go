// Package fiberlog sends one event to Logger for each request a Fiber app
// serves: method, path, status, and duration, plus the cause when the request
// failed and the operator when the app names one. Every BioTron service uses
// this one middleware so the events read the same in the log explorer.
package fiberlog

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/biotron/go/logclient"
)

// Sink receives the events. *logclient.Client satisfies it; Logger itself
// passes an adapter that writes straight into its store.
type Sink interface {
	LogAsync(level logclient.Level, message string, payload any)
}

type Options struct {
	// Quiet paths are logged only when they fail. Use it for routes a page
	// polls on every load, such as a session check.
	Quiet []string
	// Skip drops a completed request from the log when it returns true. It
	// runs after the status is known, so a service can keep an attacker's
	// refused requests from writing into the audit trail.
	Skip func(c fiber.Ctx, status int) bool
}

type localKey string

const (
	errorKey localKey = "fiberlog.error"
	actorKey localKey = "fiberlog.actor"
)

// SetError attaches the cause of a failure to the request so the completion
// event carries it. Call it where a handler writes an error status itself
// instead of returning the error; returned errors are picked up without it.
func SetError(c fiber.Ctx, err error) {
	if err != nil {
		c.Locals(errorKey, err.Error())
	}
}

// SetActor names the signed-in operator on the completion event.
func SetActor(c fiber.Ctx, login string) {
	if login != "" {
		c.Locals(actorKey, login)
	}
}

// New returns the middleware. Mount Fiber's own logger first, then this,
// then recover. Fiber's logger runs the app's error handler itself and
// returns nil, so anything mounted inside it never sees a returned error;
// recover must sit inside this so a panic is logged as the 500 it became.
func New(sink Sink, options Options) fiber.Handler {
	quiet := make(map[string]bool, len(options.Quiet))
	for _, path := range options.Quiet {
		quiet[path] = true
	}
	return func(c fiber.Ctx) error {
		started := time.Now()
		chainErr := c.Next()
		if chainErr != nil {
			if c.Locals(errorKey) == nil {
				SetError(c, chainErr)
			}
			// Run the app's error handler here, as Fiber's own logger does, so
			// the status logged is the one the client received rather than a
			// guess from the error's type. Fiber then sees no error to handle.
			if err := c.App().ErrorHandler(c, chainErr); err != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		// Method and Path point into fasthttp's pooled request buffer, and the
		// sink marshals the payload on another goroutine. Without a copy the
		// buffer can be refilled from a different request first, and the event
		// then names a path nobody chose to log.
		path := strings.Clone(c.Path())
		if path == "/health" {
			return nil
		}
		status := c.Response().StatusCode()
		if status < 400 && (c.Method() == fiber.MethodOptions || quiet[path]) {
			return nil
		}
		if options.Skip != nil && options.Skip(c, status) {
			return nil
		}

		level := logclient.Info
		if status >= 500 {
			level = logclient.Error
		} else if status >= 400 {
			level = logclient.Warning
		}
		payload := map[string]any{
			"method":      strings.Clone(c.Method()),
			"path":        path,
			"status":      status,
			"duration_ms": time.Since(started).Milliseconds(),
		}
		if actor, ok := c.Locals(actorKey).(string); ok && actor != "" {
			payload["actor"] = actor
		}
		if detail, ok := c.Locals(errorKey).(string); ok && detail != "" {
			payload["error"] = detail
		}
		sink.LogAsync(level, "HTTP request completed", payload)
		return nil
	}
}

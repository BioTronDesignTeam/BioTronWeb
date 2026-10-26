// Package logclient sends structured events to Grafana Cloud, with audit
// events also sent to PostHog.
//
// Every BioTron Go service uses this one client. The service name is passed
// in code so that the binary and its Grafana service_name label cannot drift
// apart. Each destination is switched on by its own variables and is a no-op
// without them, so a service runs unchanged where none is configured.
package logclient

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
)

type Level string

const (
	Debug   Level = "debug"
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
)

type Client struct {
	minimum int
	grafana *grafana
	posthog *posthog
}

// NewFromEnv reads LOG_LEVEL, the GRAFANA_OTLP_* variables for Grafana, and
// the POSTHOG_* variables for PostHog. BIOTRON_ENV, when set, tags every
// event with the environment that sent it. It never fails: an invalid
// LOG_LEVEL means info, and a destination missing its variables is disabled.
func NewFromEnv(service string) *Client {
	minimum, ok := severity(Level(strings.ToLower(getenv("LOG_LEVEL", string(Info)))))
	if !ok {
		log.Printf("warning: invalid LOG_LEVEL; defaulting to info")
		minimum, _ = severity(Info)
	}
	return &Client{
		minimum: minimum,
		grafana: newGrafanaFromEnv(service),
		posthog: newPostHogFromEnv(service),
	}
}

// Enabled reports whether Grafana or PostHog is configured.
func (c *Client) Enabled() bool {
	return c != nil && (c.grafana != nil || c.posthog != nil)
}

// LogAsync hands one event to the background queues and returns at once.
// Events below LOG_LEVEL are dropped before they are queued, except audit
// events, which PostHog gets whatever the level. Each queue is bounded, so a
// slow or unreachable destination can never stall the service.
func (c *Client) LogAsync(level Level, message string, payload any) {
	if !c.Enabled() {
		return
	}
	current, ok := severity(level)
	if !ok {
		log.Printf("structured log delivery failed: invalid log level %q", level)
		return
	}
	c.audit(level, message, payload)
	if current < c.minimum {
		return
	}
	if c.grafana != nil {
		c.grafana.add(level, message, payload)
	}
}

// Close sends what the background destinations still hold and stops them.
// Call it once at shutdown, after the last event, with a deadline: it waits
// until the queues are sent or ctx expires.
func (c *Client) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var errs []error
	if c.grafana != nil {
		errs = append(errs, c.grafana.batch.close(ctx))
	}
	if c.posthog != nil {
		errs = append(errs, c.posthog.batch.close(ctx))
	}
	return errors.Join(errs...)
}

// Capture sends one product event to PostHog only, for something worth
// counting that is not a log line. distinctID is the operator's login, or
// empty when the service itself acted. It never blocks and is a no-op
// without PostHog. Never put a secret or a session in the properties.
func (c *Client) Capture(distinctID, event string, properties map[string]any) {
	if c == nil || c.posthog == nil {
		return
	}
	c.posthog.add(distinctID, event, properties)
}

// audit also sends an audit event to PostHog, whatever LOG_LEVEL says: the
// level filters diagnostics, and these are a record of who did what.
func (c *Client) audit(level Level, message string, payload any) {
	if c.posthog == nil {
		return
	}
	if login, fields, ok := auditActor(level, payload); ok {
		c.posthog.add(login, message, fields)
	}
}

func severity(level Level) (int, bool) {
	switch level {
	case Debug:
		return 0, true
	case Info:
		return 1, true
	case Warning:
		return 2, true
	case Error:
		return 3, true
	default:
		return 0, false
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Package logclient sends structured events to Logger's ingest route and,
// when configured, to Grafana Cloud, with audit events also sent to PostHog.
//
// Every BioTron Go service uses this one client. The service name is passed
// in code so that the binary and its Logger catalog id cannot drift apart.
// Each destination is switched on by its own variables and is a no-op without
// them, so a service runs unchanged where none is configured.
package logclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Level string

const (
	Debug   Level = "debug"
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
)

type Client struct {
	endpoint   string
	token      string
	service    string
	minimum    int
	httpClient *http.Client
	slots      chan struct{}
	grafana    *grafana
	posthog    *posthog
}

// NewFromEnv reads LOGGER_URL, LOGGER_INGEST_TOKEN, and LOG_LEVEL for Logger,
// the GRAFANA_OTLP_* variables for Grafana, and the POSTHOG_* variables for
// PostHog. BIOTRON_ENV, when set, tags Grafana and PostHog events with the
// environment that sent them. It never fails: an invalid
// LOG_LEVEL means info, and a destination missing its variables is disabled.
func NewFromEnv(service string) *Client {
	minimum, ok := severity(Level(strings.ToLower(getenv("LOG_LEVEL", string(Info)))))
	if !ok {
		log.Printf("warning: invalid LOG_LEVEL; defaulting to info")
		minimum, _ = severity(Info)
	}
	return &Client{
		endpoint:   strings.TrimRight(getenv("LOGGER_URL", "http://logger-api:8080"), "/") + "/v1/logs",
		token:      os.Getenv("LOGGER_INGEST_TOKEN"),
		service:    service,
		minimum:    minimum,
		httpClient: &http.Client{Timeout: 2 * time.Second},
		slots:      make(chan struct{}, 32),
		grafana:    newGrafanaFromEnv(service),
		posthog:    newPostHogFromEnv(service),
	}
}

// Enabled reports whether any destination is configured.
func (c *Client) Enabled() bool {
	return c != nil && (c.token != "" || c.grafana != nil || c.posthog != nil)
}

// LoggerEnabled reports whether events reach Logger, the destination a
// service's startup warning is about.
func (c *Client) LoggerEnabled() bool {
	return c != nil && c.token != ""
}

// Log sends one event and waits for Logger's answer. Events below LOG_LEVEL
// are dropped before any request is made. Grafana and PostHog are never
// waited on: the event joins their background queues, and the error is
// Logger's alone.
func (c *Client) Log(ctx context.Context, level Level, message string, payload any) error {
	if !c.Enabled() {
		return nil
	}
	current, ok := severity(level)
	if !ok {
		return fmt.Errorf("invalid log level %q", level)
	}
	c.audit(level, message, payload)
	if current < c.minimum {
		return nil
	}
	c.export(level, message, payload)
	return c.sendToLogger(ctx, level, message, payload)
}

// LogAsync sends in the background with a two-second timeout and at most 32
// events in flight. Past that it drops the event and says so on the standard
// log, so a slow Logger can never stall the service. Grafana and PostHog have
// their own bounded queues with the same promise.
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
	c.export(level, message, payload)
	if !c.LoggerEnabled() {
		return
	}
	if c.slots != nil {
		select {
		case c.slots <- struct{}{}:
		default:
			log.Print("structured log delivery busy; dropping event")
			return
		}
	}
	go func() {
		if c.slots != nil {
			defer func() { <-c.slots }()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.sendToLogger(ctx, level, message, payload); err != nil {
			log.Printf("structured log delivery failed: %v", err)
		}
	}()
}

// Close sends what the background destinations still hold and stops them.
// Call it once at shutdown, after the last event, with a deadline: it waits
// until the queues are sent or ctx expires. Logger needs no closing.
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
	if login, ok := auditActor(level, payload); ok {
		c.posthog.add(login, message, payload.(map[string]any))
	}
}

// export hands the event to every background destination.
func (c *Client) export(level Level, message string, payload any) {
	if c.grafana != nil {
		c.grafana.add(level, message, payload)
	}
}

func (c *Client) sendToLogger(ctx context.Context, level Level, message string, payload any) error {
	if !c.LoggerEnabled() {
		return nil
	}
	body := struct {
		Service string `json:"service"`
		Level   Level  `json:"level"`
		Message string `json:"message"`
		Payload any    `json:"payload,omitempty"`
	}{Service: c.service, Level: level, Message: message, Payload: payload}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal log event: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send log event: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("logger returned HTTP %d", response.StatusCode)
	}
	return nil
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

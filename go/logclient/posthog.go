package logclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"
)

// posthog sends product and audit events to PostHog's batch capture API. It
// is not a log destination: only events that name a person doing something
// go there, never request lines or diagnostics. See auditActor for the rule.
type posthog struct {
	endpoint    string
	apiKey      string
	service     string
	environment string
	httpClient  *http.Client
	batch       *batcher[posthogEvent]
}

type posthogEvent struct {
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties"`
	Timestamp  string         `json:"timestamp"`
}

// newPostHogFromEnv reads POSTHOG_API_KEY, the project key, and POSTHOG_HOST,
// default https://us.i.posthog.com; an EU project sets
// https://eu.i.posthog.com. No key means disabled.
func newPostHogFromEnv(service string) *posthog {
	apiKey := strings.TrimSpace(os.Getenv("POSTHOG_API_KEY"))
	if apiKey == "" {
		return nil
	}
	host := strings.TrimSpace(getenv("POSTHOG_HOST", "https://us.i.posthog.com"))
	return newPostHog(host, apiKey, service, os.Getenv("BIOTRON_ENV"), defaultBatchOptions)
}

func newPostHog(host, apiKey, service, environment string, options batchOptions) *posthog {
	p := &posthog{
		endpoint:    strings.TrimRight(host, "/") + "/batch/",
		apiKey:      apiKey,
		service:     service,
		environment: environment,
		httpClient:  &http.Client{Timeout: options.timeout},
	}
	p.batch = newBatcher("PostHog event", options, p.send)
	return p
}

// add copies the properties now, on the caller's goroutine, so a map the
// caller changes afterwards cannot change what is sent. An empty distinctID
// makes the service itself the actor, without a person profile.
func (p *posthog) add(distinctID, event string, properties map[string]any) {
	copied := make(map[string]any, len(properties)+4)
	maps.Copy(copied, properties)
	copied["service"] = p.service
	copied["$lib"] = "biotron-logclient"
	if p.environment != "" {
		copied["environment"] = p.environment
	}
	if distinctID == "" {
		distinctID = p.service
		copied["$process_person_profile"] = false
	}
	p.batch.add(posthogEvent{
		Event:      event,
		DistinctID: distinctID,
		Properties: copied,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (p *posthog) send(ctx context.Context, events []posthogEvent) error {
	raw, err := json.Marshal(struct { //nolint:gosec // G117: PostHog's batch API takes the project key in the body, and that key is public by design: browsers send it too.
		APIKey string         `json:"api_key"`
		Batch  []posthogEvent `json:"batch"`
	}{p.apiKey, events})
	if err != nil {
		return fmt.Errorf("marshal PostHog batch: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send PostHog batch: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("posthog returned HTTP %d", response.StatusCode)
	}
	return nil
}

// auditActor decides which log events also go to PostHog: an info event
// whose map payload names who acted, as actor_login (admin and audit changes)
// or operator_login (sign-in and sign-out). The login becomes PostHog's
// distinct id, the message the event name, and the map its properties.
// Warnings and errors, request lines, and events with no person behind them
// stay in the logs only.
func auditActor(level Level, payload any) (string, map[string]any, bool) {
	if level != Info {
		return "", nil, false
	}
	fields, ok := payload.(map[string]any)
	if !ok {
		return "", nil, false
	}
	for _, key := range []string{"actor_login", "operator_login"} {
		if login, ok := fields[key].(string); ok && login != "" {
			return login, fields, true
		}
	}
	return "", nil, false
}

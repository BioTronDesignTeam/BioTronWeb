package logclient

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

type posthogBatch struct {
	APIKey string         `json:"api_key"`
	Batch  []posthogEvent `json:"batch"`
}

func newPostHogServer(t *testing.T, status int) (*httptest.Server, func() []posthogBatch) {
	var mu sync.Mutex
	var batches []posthogBatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/batch/" {
			t.Errorf("%s %s, want POST /batch/", r.Method, r.URL.Path)
		}
		var body posthogBatch
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		batches = append(batches, body)
		mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
		}
	}))
	t.Cleanup(server.Close)
	return server, func() []posthogBatch {
		mu.Lock()
		defer mu.Unlock()
		return batches
	}
}

func TestPostHogReceivesAuditEventsOnly(t *testing.T) {
	server, batches := newPostHogServer(t, 0)
	// LOG_LEVEL is error, and audit events still reach PostHog.
	client := &Client{service: "auth-api", minimum: 3, posthog: newPostHog(server.URL+"/", "phc_key", "auth-api", "production", testBatchOptions)}
	if !client.Enabled() || client.LoggerEnabled() {
		t.Fatal("PostHog alone must enable the client but not Logger")
	}

	payload := map[string]any{"target_login": "hubot", "actor_id": 1, "actor_login": "octocat"}
	client.LogAsync(Info, "Permission granted", payload)
	payload["target_login"] = "changed after the call"
	if err := client.Log(context.Background(), Info, "Operator signed in", map[string]any{"operator_login": "monalisa"}); err != nil {
		t.Fatal(err)
	}
	client.LogAsync(Warning, "Sign-in refused", map[string]any{"actor_login": "octocat"})
	client.LogAsync(Info, "HTTP request completed", map[string]any{"actor": "octocat", "path": "/api/x"})
	client.LogAsync(Info, "Exo API started", map[string]any{"port": "8080"})
	client.LogAsync(Info, "Not a map", struct{ ActorLogin string }{"octocat"})
	client.Capture("", "Calendar feed subscribed", map[string]any{"feed": "team"})
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := batches()
	if len(got) != 1 || got[0].APIKey != "phc_key" {
		t.Fatalf("batches = %+v", got)
	}
	events := got[0].Batch
	if len(events) != 3 {
		t.Fatalf("got %d events, want the two audit events and the capture: %+v", len(events), events)
	}
	granted := events[0]
	if granted.Event != "Permission granted" || granted.DistinctID != "octocat" || granted.Timestamp == "" {
		t.Errorf("event = %+v", granted)
	}
	if granted.Properties["target_login"] != "hubot" || granted.Properties["service"] != "auth-api" || granted.Properties["environment"] != "production" {
		t.Errorf("properties = %+v", granted.Properties)
	}
	if events[1].Event != "Operator signed in" || events[1].DistinctID != "monalisa" {
		t.Errorf("event = %+v", events[1])
	}
	captured := events[2]
	if captured.DistinctID != "auth-api" || captured.Properties["$process_person_profile"] != false || captured.Properties["feed"] != "team" {
		t.Errorf("a capture with no actor must be the service's, without a person: %+v", captured)
	}
}

func TestPostHogFailureNeverReachesTheCaller(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	server, batches := newPostHogServer(t, http.StatusServiceUnavailable)
	client := &Client{minimum: 1, posthog: newPostHog(server.URL, "phc_key", "auth-api", "", testBatchOptions)}
	if err := client.Log(context.Background(), Info, "Operator signed out", map[string]any{"operator_login": "octocat"}); err != nil {
		t.Fatalf("a PostHog failure must not fail Log: %v", err)
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(batches()) != 1 {
		t.Fatal("the event was never attempted")
	}
}

func TestPostHogFromEnv(t *testing.T) {
	t.Setenv("POSTHOG_API_KEY", "")
	if newPostHogFromEnv("auth-api") != nil {
		t.Fatal("no key must disable PostHog")
	}
	var nilClient *Client
	nilClient.Capture("octocat", "ignored", nil)
	(&Client{}).Capture("octocat", "ignored", nil)

	t.Setenv("POSTHOG_API_KEY", "phc_key")
	t.Setenv("POSTHOG_HOST", "")
	p := newPostHogFromEnv("auth-api")
	if p == nil || p.endpoint != "https://us.i.posthog.com/batch/" {
		t.Fatalf("posthog = %+v", p)
	}
	_ = p.batch.close(context.Background())
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com/")
	p = newPostHogFromEnv("auth-api")
	if p.endpoint != "https://eu.i.posthog.com/batch/" {
		t.Fatalf("endpoint = %q", p.endpoint)
	}
	_ = p.batch.close(context.Background())
}

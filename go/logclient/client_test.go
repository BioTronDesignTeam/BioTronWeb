package logclient

import (
	"context"
	"testing"
)

func TestDisabledClientIsNoop(t *testing.T) {
	var nilClient *Client
	nilClient.LogAsync(Info, "ignored", nil)
	if err := nilClient.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := &Client{}
	if client.Enabled() {
		t.Fatal("a client with no destination must be disabled")
	}
	client.LogAsync(Info, "ignored", nil)
}

func TestNewFromEnvNeverFails(t *testing.T) {
	t.Setenv("GRAFANA_OTLP_ENDPOINT", "")
	t.Setenv("POSTHOG_API_KEY", "")
	t.Setenv("LOG_LEVEL", "verbose")
	client := NewFromEnv("calendar-api")
	if client.Enabled() {
		t.Fatal("a client with neither Grafana nor PostHog must be disabled")
	}
	if client.minimum != 1 {
		t.Fatalf("minimum = %d, want info (1) for an invalid LOG_LEVEL", client.minimum)
	}

	t.Setenv("POSTHOG_API_KEY", "phc_key")
	t.Setenv("LOG_LEVEL", "WARNING")
	client = NewFromEnv("calendar-api")
	defer func() { _ = client.Close(context.Background()) }()
	if !client.Enabled() || client.minimum != 2 {
		t.Fatalf("unexpected client: %+v", client)
	}
}

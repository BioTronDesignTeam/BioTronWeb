package logclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSendsStructuredEventAndFiltersMinimumLevel(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing bearer token")
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["service"] != "exo-api" || body["level"] != "error" {
			t.Errorf("unexpected body: %#v", body)
		}
		response.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := &Client{endpoint: server.URL, token: "secret", service: "exo-api", minimum: 2, httpClient: server.Client()}
	if err := client.Log(context.Background(), Info, "filtered", nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Log(context.Background(), Error, "sent", map[string]int{"status": 500}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestDisabledClientIsNoop(t *testing.T) {
	var nilClient *Client
	if err := nilClient.Log(context.Background(), Info, "ignored", nil); err != nil {
		t.Fatal(err)
	}
	client := &Client{}
	if err := client.Log(context.Background(), Info, "ignored", nil); err != nil {
		t.Fatal(err)
	}
	client.LogAsync(Info, "ignored", nil)
}

func TestNewFromEnvNeverFails(t *testing.T) {
	t.Setenv("LOGGER_INGEST_TOKEN", "")
	t.Setenv("LOG_LEVEL", "verbose")
	client := NewFromEnv("calendar-api")
	if client.Enabled() {
		t.Fatal("client with an empty token must be disabled")
	}
	if client.minimum != 1 {
		t.Fatalf("minimum = %d, want info (1) for an invalid LOG_LEVEL", client.minimum)
	}

	t.Setenv("LOGGER_INGEST_TOKEN", "secret")
	t.Setenv("LOGGER_URL", "http://logger/")
	t.Setenv("LOG_LEVEL", "WARNING")
	client = NewFromEnv("calendar-api")
	if !client.Enabled() || client.endpoint != "http://logger/v1/logs" || client.minimum != 2 || client.service != "calendar-api" {
		t.Fatalf("unexpected client: %+v", client)
	}
}

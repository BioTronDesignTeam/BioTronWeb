package eventlog

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
			t.Fatal("missing bearer token")
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["service"] != "oauth-manager" || body["level"] != "error" {
			t.Fatalf("unexpected body: %#v", body)
		}
		response.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := &Client{endpoint: server.URL, token: "secret", service: "oauth-manager", minimum: 2, httpClient: server.Client()}
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
	client := &Client{}
	if err := client.Log(context.Background(), Info, "ignored", nil); err != nil {
		t.Fatal(err)
	}
}

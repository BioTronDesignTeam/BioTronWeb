package logger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMinimumLevelFiltersAtSource(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing bearer token")
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["service"] != "exo-api" || body["level"] != "error" {
			t.Fatalf("unexpected body: %#v", body)
		}
		response.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, err := New(Config{URL: server.URL, Token: "secret", Service: "exo-api", Level: Warning})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.LogInfo(context.Background(), "filtered", nil); err != nil {
		t.Fatal(err)
	}
	if err := client.LogError(context.Background(), "sent", map[string]int{"code": 7}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestRejectsInvalidMinimumLevel(t *testing.T) {
	_, err := New(Config{URL: "http://logger", Token: "secret", Service: "api", Level: "verbose"})
	if err == nil {
		t.Fatal("expected invalid level error")
	}
}

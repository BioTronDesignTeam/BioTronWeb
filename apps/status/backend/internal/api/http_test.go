package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/catalog"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/model"
)

// These pin the plain net/http behaviour of the API: ServeMux's own answers
// for paths and methods it does not route, the JSON error a handler returns,
// and the limiter's refusal.

func plainApp(t *testing.T, options Options) http.Handler {
	t.Helper()
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return New(&fakeStore{}, serviceCatalog, options)
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	app := plainApp(t, Options{})
	for _, path := range []string{"/nope", "/v1", "/v1/nope", "/v1/status/", "/V1/STATUS"} {
		if response := serve(app, httptest.NewRequest(http.MethodGet, path, nil)); response.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, response.Code)
		}
	}
}

func TestWrongMethodIsMethodNotAllowed(t *testing.T) {
	app := plainApp(t, Options{})
	for _, target := range []struct{ method, path string }{
		{http.MethodPost, "/v1/status"},
		{http.MethodDelete, "/health"},
		{http.MethodPut, "/v1/status/history"},
	} {
		response := serve(app, httptest.NewRequest(target.method, target.path, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("%s %s = %d, Allow %q", target.method, target.path, response.Code, response.Header().Get("Allow"))
		}
	}
}

// A refusal a handler returns is JSON, the one shape the status page reads.
type failingStore struct{ fakeStore }

func (failingStore) LatestHealth(context.Context, []string) (map[string]model.Health, error) {
	return nil, errors.New("database is down")
}

func TestHandlerErrorsAreJSON(t *testing.T) {
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	response := serve(New(&failingStore{}, serviceCatalog, Options{}), httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("store down = %d, want 503", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body["error"] == "" {
		t.Fatalf("body = %v, %v", body, err)
	}
}

func TestLimiterRefusesWithRetryAfter(t *testing.T) {
	app := plainApp(t, Options{StatusRateLimit: 2})
	get := func() *httptest.ResponseRecorder {
		return serve(app, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	}
	for range 2 {
		if response := get(); response.Code != http.StatusOK {
			t.Fatalf("request under the limit = %d", response.Code)
		}
	}
	third := get()
	if third.Code != http.StatusTooManyRequests || third.Header().Get("Retry-After") == "" {
		t.Fatalf("third = %d, Retry-After %q", third.Code, third.Header().Get("Retry-After"))
	}
	body, _ := io.ReadAll(third.Body)
	if string(body) != "Too Many Requests\n" {
		t.Fatalf("429 body = %q", body)
	}
}

// The public limiter covers the status routes, never /health, which the
// monitor and the container healthcheck poll.
func TestPublicLimitSparesHealth(t *testing.T) {
	app := plainApp(t, Options{StatusRateLimit: 1})
	serve(app, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if response := serve(app, httptest.NewRequest(http.MethodGet, "/v1/status/history", nil)); response.Code != http.StatusTooManyRequests {
		t.Fatalf("history after the bucket emptied = %d, want 429", response.Code)
	}
	if response := serve(app, httptest.NewRequest(http.MethodGet, "/health", nil)); response.Code != http.StatusOK {
		t.Fatalf("health = %d, want 200", response.Code)
	}
}

// A trusted proxy that sends no usable Cf-Connecting-Ip does not pool every
// such request into one bucket: the peer address is the key.
func TestTrustedPeerWithoutHeaderIsKeyedByPeer(t *testing.T) {
	trust := newProxyTrust([]string{"192.0.2.1"})
	for _, header := range []string{"", "not-an-ip"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
		request.RemoteAddr = "192.0.2.1:4000"
		if header != "" {
			request.Header.Set("Cf-Connecting-Ip", header)
		}
		if got := trust.clientIP(request); got != "192.0.2.1" {
			t.Fatalf("Cf-Connecting-Ip %q: key = %q, want the peer", header, got)
		}
	}
}

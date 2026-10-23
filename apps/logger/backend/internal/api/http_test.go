package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"

	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
)

// These pin what the move from Fiber to net/http must not change: the shape
// of every error, the headers the limiters send, and the routes that Fiber's
// lenient matching accepted.

func compatApp(t *testing.T, options Options) http.Handler {
	t.Helper()
	serviceCatalog, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if options.IngestToken == "" {
		options.IngestToken = "secret"
	}
	return New(&fakeStore{}, serviceCatalog, auth.AllowAll{}, options)
}

func errorBody(t *testing.T, response *http.Response) string {
	t.Helper()
	if got := response.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body["error"]
}

func TestUnknownRouteIsAJSONNotFound(t *testing.T) {
	app := compatApp(t, Options{})
	for _, path := range []string{"/nope", "/v1", "/v1/nope", "/v1/apps/nope/logs/recent"} {
		response := serve(app, httptest.NewRequest(http.MethodGet, path, nil))
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, response.StatusCode)
		}
		if got := errorBody(t, response); got != "Not Found" {
			t.Fatalf("GET %s error = %q", path, got)
		}
	}
}

func TestWrongMethodIsAJSONMethodNotAllowed(t *testing.T) {
	app := compatApp(t, Options{})
	cases := map[string]string{
		"GET /v1/logs":                  "POST",
		"POST /v1/status":               "GET, HEAD",
		"DELETE /health":                "GET, HEAD",
		"POST /v1/apps/exo/logs/recent": "GET, HEAD",
	}
	for request, allow := range cases {
		method, path, _ := strings.Cut(request, " ")
		response := serve(app, httptest.NewRequest(method, path, nil))
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d, want 405", request, response.StatusCode)
		}
		if got := response.Header.Get("Allow"); got != allow {
			t.Fatalf("%s Allow = %q, want %q", request, got, allow)
		}
		if got := errorBody(t, response); got != "Method Not Allowed" {
			t.Fatalf("%s error = %q", request, got)
		}
	}
}

func TestTrailingSlashReachesTheRoute(t *testing.T) {
	app := compatApp(t, Options{})
	response := serve(app, httptest.NewRequest(http.MethodGet, "/v1/session/", nil))
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /v1/session/ = %d, Cache-Control %q", response.StatusCode, response.Header.Get("Cache-Control"))
	}
}

func TestErrorsUseOneJSONShape(t *testing.T) {
	app := compatApp(t, Options{})

	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := serve(app, request)
	if response.StatusCode != http.StatusUnauthorized || errorBody(t, response) != "Unauthorized" {
		t.Fatalf("ingest without a token = %d", response.StatusCode)
	}

	response = serve(app, httptest.NewRequest(http.MethodGet, "/v1/apps/exo/logs/history?from=yesterday", nil))
	if response.StatusCode != http.StatusBadRequest || errorBody(t, response) != "from must be an RFC3339 timestamp" {
		t.Fatalf("bad from = %d", response.StatusCode)
	}
}

func TestIngestNeedsAJSONBodyWithinTheLimit(t *testing.T) {
	app := compatApp(t, Options{})
	post := func(contentType string, body []byte) *http.Response {
		request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer secret")
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		return serve(app, request)
	}
	event := []byte(`{"service":"exo-api","level":"info","message":"ok"}`)

	if response := post("application/json; charset=utf-8", event); response.StatusCode != http.StatusCreated {
		t.Fatalf("JSON with a charset = %d, want 201", response.StatusCode)
	}
	for _, contentType := range []string{"", "text/plain"} {
		response := post(contentType, event)
		if response.StatusCode != http.StatusBadRequest || errorBody(t, response) != "invalid JSON body" {
			t.Fatalf("Content-Type %q = %d, want 400", contentType, response.StatusCode)
		}
	}
	if response := post("application/json", []byte(`{"service":`)); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("truncated JSON = %d, want 400", response.StatusCode)
	}

	huge := []byte(`{"service":"exo-api","level":"info","message":"` + strings.Repeat("x", bodyLimit) + `"}`)
	response := post("application/json", huge)
	if response.StatusCode != http.StatusRequestEntityTooLarge || errorBody(t, response) != "Request Entity Too Large" {
		t.Fatalf("oversized body = %d, want 413", response.StatusCode)
	}
}

func TestLimiterSendsRateLimitHeaders(t *testing.T) {
	app := compatApp(t, Options{StatusRateLimit: 2})
	get := func() *http.Response {
		return serve(app, httptest.NewRequest(http.MethodGet, "/v1/session", nil))
	}

	first := get()
	if first.Header.Get("X-RateLimit-Limit") != "2" || first.Header.Get("X-RateLimit-Remaining") != "1" {
		t.Fatalf("first headers = %v", first.Header)
	}
	if reset := first.Header.Get("X-RateLimit-Reset"); reset == "" || reset == "0" {
		t.Fatalf("X-RateLimit-Reset = %q", reset)
	}
	if second := get(); second.Header.Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("second headers = %v", second.Header)
	}

	third := get()
	if third.StatusCode != http.StatusTooManyRequests || third.Header.Get("Retry-After") == "" {
		t.Fatalf("third = %d, Retry-After %q", third.StatusCode, third.Header.Get("Retry-After"))
	}
	body, _ := io.ReadAll(third.Body)
	if string(body) != "Too Many Requests" {
		t.Fatalf("429 body = %q", body)
	}
}

// Fiber mounted the public limiter on all of /v1 after the ingest route, so
// the log explorer shares the status page's bucket and ingest does not.
func TestPublicLimitCoversTheExplorerButNotIngest(t *testing.T) {
	app := compatApp(t, Options{StatusRateLimit: 1})
	serve(app, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if response := serve(app, httptest.NewRequest(http.MethodGet, "/v1/apps", nil)); response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("explorer after the status bucket emptied = %d, want 429", response.StatusCode)
	}
	if response := serve(app, httptest.NewRequest(http.MethodGet, "/health", nil)); response.StatusCode != http.StatusOK {
		t.Fatalf("health = %d, want 200", response.StatusCode)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(`{"service":"exo-api","level":"info","message":"ok"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer secret")
	if response := serve(app, request); response.StatusCode != http.StatusCreated {
		t.Fatalf("ingest = %d, want 201", response.StatusCode)
	}
}

func TestPanicIsAJSONInternalErrorInTheRequestLog(t *testing.T) {
	events := &eventCollector{}
	handler := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	app := httplog.New(events, httplog.Options{})(handler)

	response := serve(app, httptest.NewRequest(http.MethodGet, "/v1/apps", nil))
	if response.StatusCode != http.StatusInternalServerError || errorBody(t, response) != "internal server error" {
		t.Fatalf("panic = %d", response.StatusCode)
	}
	if len(events.events) != 1 || events.events[0].level != logclient.Error || events.events[0].payload["error"] != "boom" {
		t.Fatalf("events = %#v, want one error naming the panic", events.events)
	}
}

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// routingApp is the real handler with no store behind it, enough for every
// answer that is decided before a query runs.
func routingApp(t *testing.T) http.Handler {
	t.Helper()
	return New(
		config.Config{FrontendURL: "http://localhost:5176", SiteURL: "http://localhost:5177"},
		nil, editorAuth(t).Client(AppID), logclient.NewFromEnv("calendar-api"), time.UTC,
	)
}

type answer struct {
	status  int
	headers map[string]string
	body    string
}

func checkAnswer(t *testing.T, app http.Handler, request *http.Request, want answer) {
	t.Helper()
	response := serveTest(app, request)
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != want.status {
		t.Fatalf("%s %s returned %d, want %d: %s", request.Method, request.URL, response.StatusCode, want.status, body)
	}
	for name, value := range want.headers {
		if got := response.Header.Get(name); got != value {
			t.Fatalf("%s %s sent %s %q, want %q", request.Method, request.URL, name, got, value)
		}
	}
	if string(body) != want.body {
		t.Fatalf("%s %s sent %q, want %q", request.Method, request.URL, body, want.body)
	}
}

// The answers below are what the Fiber app sent, captured from it before the
// move to net/http. Browsers, the site, and feed readers saw these exact
// statuses, headers, and bodies.
func TestRoutingAnswersAsFiberDid(t *testing.T) {
	app := routingApp(t)
	jsonType := "application/json; charset=utf-8"

	checkAnswer(t, app, httptest.NewRequest(http.MethodGet, "/v1/nope", nil), answer{
		status: 404, headers: map[string]string{"Content-Type": jsonType, "Vary": ""},
		body: `{"error":"not found"}`,
	})
	checkAnswer(t, app, httptest.NewRequest(http.MethodGet, "//v1/scopes", nil), answer{
		status: 404, body: `{"error":"not found"}`,
	})
	checkAnswer(t, app, httptest.NewRequest(http.MethodPost, "/health", nil), answer{
		status: 405, headers: map[string]string{"Allow": "GET, HEAD", "Content-Type": jsonType},
		body: `{"error":"method not allowed"}`,
	})
	checkAnswer(t, app, httptest.NewRequest(http.MethodPost, "/v1/scopes/", nil), answer{
		status: 405, headers: map[string]string{"Allow": "GET, HEAD, OPTIONS"},
		body: `{"error":"method not allowed"}`,
	})
	checkAnswer(t, app, httptest.NewRequest(http.MethodGet, "/v1/feeds/scopes/abc", nil), answer{
		status: 405, headers: map[string]string{"Allow": "OPTIONS"},
		body: `{"error":"method not allowed"}`,
	})
	// A non-preflight OPTIONS falls through CORS to nothing.
	checkAnswer(t, app, httptest.NewRequest(http.MethodOptions, "/v1/scopes", nil), answer{
		status: 404, headers: map[string]string{"Vary": "Origin"}, body: `{"error":"not found"}`,
	})
}

func TestCORSAnswersAsFiberDid(t *testing.T) {
	app := routingApp(t)

	preflight := httptest.NewRequest(http.MethodOptions, "/v1/feeds/all.ics", nil)
	preflight.Header.Set("Origin", "http://localhost:5177")
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	checkAnswer(t, app, preflight, answer{status: 204, headers: map[string]string{
		"Access-Control-Allow-Origin":      "http://localhost:5177",
		"Access-Control-Allow-Methods":     "GET, HEAD, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type",
		"Access-Control-Allow-Credentials": "",
		"Vary":                             "Access-Control-Request-Method, Access-Control-Request-Headers, Origin",
		"Content-Type":                     "",
	}})

	// A stranger still gets a 204, only without its origin echoed back.
	stranger := httptest.NewRequest(http.MethodOptions, "/v1/scopes", nil)
	stranger.Header.Set("Origin", "https://evil.example")
	stranger.Header.Set("Access-Control-Request-Method", "GET")
	checkAnswer(t, app, stranger, answer{status: 204, headers: map[string]string{
		"Access-Control-Allow-Origin":  "",
		"Access-Control-Allow-Methods": "GET, HEAD, OPTIONS",
	}})

	// The site may read the public calendar but is not an editor origin.
	admin := httptest.NewRequest(http.MethodOptions, "/v1/admin/scopes", nil)
	admin.Header.Set("Origin", "http://localhost:5176")
	admin.Header.Set("Access-Control-Request-Method", "POST")
	checkAnswer(t, app, admin, answer{status: 204, headers: map[string]string{
		"Access-Control-Allow-Origin":      "http://localhost:5176",
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Allow-Methods":     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type, X-Requested-With",
	}})
	site := httptest.NewRequest(http.MethodOptions, "/v1/auth/status", nil)
	site.Header.Set("Origin", "http://localhost:5177")
	site.Header.Set("Access-Control-Request-Method", "GET")
	checkAnswer(t, app, site, answer{status: 204, headers: map[string]string{
		"Access-Control-Allow-Origin": "", "Access-Control-Allow-Credentials": "",
	}})

	status := httptest.NewRequest(http.MethodGet, "/v1/auth/status", nil)
	status.Header.Set("Origin", "http://localhost:5176")
	status.Header.Set("Cookie", "session=abc")
	checkAnswer(t, app, status, answer{
		status: 200,
		headers: map[string]string{
			"Access-Control-Allow-Origin": "http://localhost:5176", "Access-Control-Allow-Credentials": "true",
			"Vary": "Origin", "Content-Type": "application/json; charset=utf-8",
		},
		body: `{"operator":{"github_id":4242,"login":"ada","name":"Ada Lovelace","avatar_url":"","is_superuser":false,"is_manager":false,"is_staff":false,"is_guest":false,"guest_app_id":""},"can_write":true,"permissions":{"write":true}}`,
	})
}

// Every admin path, known or not, is behind the write check, as it was under
// Fiber's group middleware; an editor then gets the router's answer.
func TestAdminPathsAreCheckedBeforeTheyAreRouted(t *testing.T) {
	app := routingApp(t)

	checkAnswer(t, app, httptest.NewRequest(http.MethodGet, "/v1/admin/nope", nil), answer{
		status: 401, body: `{"error":"unauthorized"}`,
	})
	checkAnswer(t, app, editorRequest(http.MethodGet, "/v1/admin/nope"), answer{
		status: 405, headers: map[string]string{"Allow": "OPTIONS"}, body: `{"error":"method not allowed"}`,
	})
	checkAnswer(t, app, editorRequest(http.MethodPut, "/v1/admin/scopes"), answer{
		status: 405, headers: map[string]string{"Allow": "GET, HEAD, POST, OPTIONS"}, body: `{"error":"method not allowed"}`,
	})
	checkAnswer(t, app, editorRequest(http.MethodOptions, "/v1/admin/scopes"), answer{
		status: 404, body: `{"error":"not found"}`,
	})
}

func TestIsJSONReadsContentTypeAsFiberDid(t *testing.T) {
	for _, contentType := range []string{"application/json", "Application/JSON; charset=utf-8", "application/vnd.api+json"} {
		if !isJSON(contentType) {
			t.Fatalf("%q should count as JSON", contentType)
		}
	}
	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded", "text/json"} {
		if isJSON(contentType) {
			t.Fatalf("%q should not count as JSON", contentType)
		}
	}
}

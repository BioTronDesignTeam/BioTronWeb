package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient/authclienttest"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// authAs returns a client whose stand-in Auth signs every cookie in as
// octocat, holding the permissions granted.
func authAs(t *testing.T, granted ...string) *authclient.Client {
	t.Helper()
	stub := authclienttest.New(t)
	stub.SignIn(authclient.Operator{GitHubID: 7, Login: "octocat"}, granted...)
	return stub.Client(AppID)
}

func adminAuth(t *testing.T) *authclient.Client { return authAs(t, PermissionAdmin) }

// authAnswering returns a client whose stand-in Auth answers every cookie with
// status.
func authAnswering(t *testing.T, status int) *authclient.Client {
	t.Helper()
	stub := authclienttest.New(t)
	stub.Answer(status)
	return stub.Client(AppID)
}

// fakeStore answers from memory and records the last write, which is all the
// gate and validation tests need.
type fakeStore struct {
	pingErr    error
	guards     []store.Guard
	guardErr   error
	upserted   *store.Guard
	created    *store.Automation
	automation store.Automation
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

func (f *fakeStore) ListGuards(context.Context) ([]store.Guard, error) {
	return f.guards, f.guardErr
}

func (f *fakeStore) GetGuard(_ context.Context, subject string) (store.Guard, error) {
	for _, guard := range f.guards {
		if guard.Subject == subject {
			return guard, nil
		}
	}
	return store.Guard{}, store.ErrNotFound
}

func (f *fakeStore) UpsertGuard(_ context.Context, guard store.Guard) (store.Guard, error) {
	f.upserted = &guard
	return guard, nil
}

func (f *fakeStore) DeleteGuard(context.Context, string) error { return nil }

func (f *fakeStore) ListAutomations(context.Context) ([]store.Automation, error) {
	return []store.Automation{}, nil
}

func (f *fakeStore) GetAutomation(context.Context, string) (store.Automation, error) {
	return f.automation, nil
}

func (f *fakeStore) CreateAutomation(_ context.Context, automation store.Automation) (store.Automation, error) {
	f.created = &automation
	return automation, nil
}

func (f *fakeStore) UpdateAutomation(_ context.Context, automation store.Automation) (store.Automation, error) {
	return automation, nil
}

func (f *fakeStore) DeleteAutomation(context.Context, string) error { return nil }

func newTestApp(t *testing.T, authz *authclient.Client, sprinterStore Store) *testApp {
	t.Helper()
	return &testApp{t: t, app: New(config.Config{}, sprinterStore, authz, &logclient.Client{}, nil)}
}

type testApp struct {
	t   *testing.T
	app http.Handler
}

func (f *testApp) do(method, path, body string, headers map[string]string) (int, string) {
	f.t.Helper()
	status, _, raw := f.doHeaders(method, path, body, headers)
	return status, raw
}

func (f *testApp) doHeaders(method, path, body string, headers map[string]string) (int, http.Header, string) {
	f.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	f.app.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, string(raw)
}

const cookie = "biotron_session=abc"

var xhr = map[string]string{"X-Requested-With": "XMLHttpRequest"}

func withCookie(extra map[string]string) map[string]string {
	headers := map[string]string{"Cookie": cookie}
	for key, value := range extra {
		headers[key] = value
	}
	return headers
}

// The gate is the whole security story of this API, so every branch of it is
// a row here rather than a comment.
func TestRequireAdminMapsEveryOutcome(t *testing.T) {
	cases := []struct {
		name       string
		authorizer *authclient.Client
		method     string
		headers    map[string]string
		want       int
	}{
		{
			name:       "no cookie is 401",
			authorizer: authAnswering(t, http.StatusUnauthorized),
			method:     http.MethodGet,
			want:       http.StatusUnauthorized,
		},
		{
			name:       "rejected session is 401",
			authorizer: authAnswering(t, http.StatusUnauthorized),
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusUnauthorized,
		},
		{
			name:       "a guest of another tool is 401",
			authorizer: authAnswering(t, http.StatusForbidden),
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusUnauthorized,
		},
		{
			name:       "signed in without permission is 403",
			authorizer: authAs(t),
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusForbidden,
		},
		{
			name:       "a broken Auth is 503, never a pass",
			authorizer: authAnswering(t, http.StatusInternalServerError),
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusServiceUnavailable,
		},
		{
			name:       "a write without X-Requested-With is 403",
			authorizer: adminAuth(t),
			method:     http.MethodDelete,
			headers:    withCookie(nil),
			want:       http.StatusForbidden,
		},
		{
			name:       "an admin read passes",
			authorizer: adminAuth(t),
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusOK,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			app := newTestApp(t, testCase.authorizer, &fakeStore{guards: []store.Guard{}})
			path := "/v1/admin/guards"
			if testCase.method == http.MethodDelete {
				path = "/v1/admin/guards/agent"
			}
			status, body := app.do(testCase.method, path, "", testCase.headers)
			if status != testCase.want {
				t.Fatalf("status = %d, want %d (%s)", status, testCase.want, body)
			}
		})
	}
}

func TestPutGuardStoresAndValidates(t *testing.T) {
	sprinterStore := &fakeStore{}
	app := newTestApp(t, adminAuth(t), sprinterStore)

	body := `{"guild_id":"123456789012345678","role_ids":["223456789012345678"],"channel_ids":[]}`
	status, response := app.do(http.MethodPut, "/v1/admin/guards/agent", body, withCookie(xhr))
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, response)
	}
	if sprinterStore.upserted == nil || sprinterStore.upserted.Subject != "agent" {
		t.Fatalf("guard not stored: %+v", sprinterStore.upserted)
	}
	var saved store.Guard
	if err := json.Unmarshal([]byte(response), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.GuildID != "123456789012345678" || len(saved.RoleIDs) != 1 {
		t.Fatalf("saved = %+v", saved)
	}

	// An empty role list would admit nobody; a non-numeric id is not a
	// snowflake; an unknown subject is a typo, not a new feature.
	rejects := []struct {
		name, path, body string
	}{
		{"empty roles", "/v1/admin/guards/agent", `{"guild_id":"123456789012345678","role_ids":[]}`},
		{"bad guild", "/v1/admin/guards/agent", `{"guild_id":"nope","role_ids":["223456789012345678"]}`},
		{"bad role", "/v1/admin/guards/agent", `{"guild_id":"123456789012345678","role_ids":["nope"]}`},
		{"unknown subject", "/v1/admin/guards/whatever", `{"guild_id":"123456789012345678","role_ids":["223456789012345678"]}`},
		{"not JSON", "/v1/admin/guards/agent", `nonsense`},
	}
	for _, reject := range rejects {
		t.Run(reject.name, func(t *testing.T) {
			status, body := app.do(http.MethodPut, reject.path, reject.body, withCookie(xhr))
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", status, body)
			}
		})
	}
}

func TestCreateAutomationValidatesBody(t *testing.T) {
	sprinterStore := &fakeStore{}
	app := newTestApp(t, adminAuth(t), sprinterStore)

	good := `{"kind":"ANNOUNCE","name":"Weekly meetings","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234",` +
		`"channel_id":"323456789012345678","deliver":"CHANNEL","post_hour":9}`
	status, body := app.do(http.MethodPost, "/v1/admin/automations", good, withCookie(xhr))
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", status, body)
	}
	if sprinterStore.created == nil || sprinterStore.created.LeadHours != 24 || !sprinterStore.created.Enabled {
		t.Fatalf("defaults not applied: %+v", sprinterStore.created)
	}

	rejects := map[string]string{
		"unknown kind":    `{"kind":"SHOUT","name":"x","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234","channel_id":"323456789012345678"}`,
		"unknown deliver": `{"kind":"NUDGE","name":"x","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234","channel_id":"323456789012345678","deliver":"SMS"}`,
		"scope not uuid":  `{"kind":"ANNOUNCE","name":"x","scope_id":"nope","channel_id":"323456789012345678"}`,
		"channel not id":  `{"kind":"ANNOUNCE","name":"x","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234","channel_id":"nope"}`,
		"lead too long":   `{"kind":"ANNOUNCE","name":"x","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234","channel_id":"323456789012345678","lead_hours":900}`,
		"post hour":       `{"kind":"ANNOUNCE","name":"x","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234","channel_id":"323456789012345678","post_hour":25}`,
		"dm without lead": `{"kind":"NUDGE","name":"x","scope_id":"8f1a0c62-1f1e-4a1d-9c4f-5b2f0f4d1234","channel_id":"323456789012345678","deliver":"DM"}`,
		"missing fields":  `{"kind":"ANNOUNCE"}`,
	}
	for name, body := range rejects {
		t.Run(name, func(t *testing.T) {
			status, response := app.do(http.MethodPost, "/v1/admin/automations", body, withCookie(xhr))
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", status, response)
			}
		})
	}
}

func TestHealthReportsDatabaseAndDiscord(t *testing.T) {
	app := newTestApp(t, authAnswering(t, http.StatusUnauthorized), &fakeStore{})
	status, body := app.do(http.MethodGet, "/health", "", nil)
	if status != http.StatusOK || !strings.Contains(body, `"database":"ok"`) {
		t.Fatalf("status = %d body = %s", status, body)
	}
	if !strings.Contains(body, `"discord_connected":false`) {
		t.Fatalf("body = %s", body)
	}

	if !strings.Contains(body, `"discord_enabled":false`) {
		t.Fatalf("body = %s", body)
	}

	down := newTestApp(t, authAnswering(t, http.StatusUnauthorized), &fakeStore{pingErr: errors.New("no connection")})
	status, body = down.do(http.MethodGet, "/health", "", nil)
	if status != http.StatusServiceUnavailable || !strings.Contains(body, `"database":"unavailable"`) {
		t.Fatalf("status = %d body = %s", status, body)
	}
}

// With Discord off the admin API keeps working, and both the health probe and
// the page's status call say so, so nobody mistakes "off" for "broken".
func TestDiscordFlagIsReported(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		app := &testApp{t: t, app: New(
			config.Config{Discord: enabled}, &fakeStore{guards: []store.Guard{}},
			adminAuth(t), &logclient.Client{}, nil,
		)}
		want := `"discord_enabled":` + map[bool]string{true: "true", false: "false"}[enabled]

		status, body := app.do(http.MethodGet, "/health", "", nil)
		if status != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("health: status = %d body = %s", status, body)
		}
		status, body = app.do(http.MethodGet, "/v1/auth/status", "", withCookie(nil))
		if status != http.StatusOK || !strings.Contains(body, want) || !strings.Contains(body, `"can_admin":true`) {
			t.Fatalf("auth status: status = %d body = %s", status, body)
		}
		status, body = app.do(http.MethodGet, "/v1/admin/guards", "", withCookie(nil))
		if status != http.StatusOK {
			t.Fatalf("guards: status = %d body = %s", status, body)
		}
	}
}

// The routes answer exactly as the Fiber version did, quirks included, so the
// admin page and anything that probes the API see no change.
func TestRoutingMatchesTheOldServer(t *testing.T) {
	origins := config.Config{FrontendURL: "http://localhost:5178"}
	app := &testApp{t: t, app: New(origins, &fakeStore{guards: []store.Guard{}},
		adminAuth(t), &logclient.Client{}, nil)}
	origin := map[string]string{"Origin": "http://localhost:5178"}
	preflight := map[string]string{"Origin": "http://localhost:5178", "Access-Control-Request-Method": "PUT"}
	cases := []struct {
		name, method, path string
		headers            map[string]string
		status             int
		body               string
		header             map[string]string
	}{
		{name: "unknown path", method: http.MethodGet, path: "/nope", status: 404, body: `{"error":"not found"}`},
		{name: "trailing slash", method: http.MethodGet, path: "/health/", status: 200},
		{name: "any case", method: http.MethodGet, path: "/HEALTH", status: 200},
		{
			name: "wrong method on health", method: http.MethodPost, path: "/health", status: 405,
			body: `{"error":"method not allowed"}`, header: map[string]string{"Allow": "GET, HEAD"},
		},
		{
			name: "wrong method on status", method: http.MethodPost, path: "/v1/auth/status", status: 405,
			header: map[string]string{"Allow": "GET, HEAD, OPTIONS"},
		},
		{
			name: "status with an allowed origin", method: http.MethodGet, path: "/v1/auth/status", headers: origin,
			status: 200, header: map[string]string{
				"Vary": "Origin", "Access-Control-Allow-Origin": "http://localhost:5178",
				"Access-Control-Allow-Credentials": "true", "Content-Type": "application/json; charset=utf-8",
			},
		},
		{
			name: "status with another origin", method: http.MethodGet, path: "/v1/auth/status",
			headers: map[string]string{"Origin": "http://evil.example"}, status: 200,
			header: map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": ""},
		},
		{
			name: "preflight", method: http.MethodOptions, path: "/v1/admin/guards/agent", headers: preflight,
			status: 204, header: map[string]string{
				"Vary":                         "Access-Control-Request-Method, Access-Control-Request-Headers, Origin",
				"Access-Control-Allow-Origin":  "http://localhost:5178",
				"Access-Control-Allow-Methods": "GET, POST, PUT, PATCH, DELETE, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, X-Requested-With",
			},
		},
		{
			name: "an OPTIONS that is not a preflight", method: http.MethodOptions, path: "/v1/auth/status",
			headers: origin, status: 404, header: map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": ""},
		},
		{
			name: "admin path with a wrong method", method: http.MethodPost, path: "/v1/admin/guards/agent",
			headers: withCookie(xhr), status: 405, header: map[string]string{"Allow": "GET, HEAD, PUT, DELETE, OPTIONS"},
		},
		{
			name: "unknown admin path", method: http.MethodGet, path: "/v1/admin/nope",
			headers: withCookie(nil), status: 405, header: map[string]string{"Allow": "OPTIONS"},
		},
		{name: "admin list with a trailing slash", method: http.MethodGet, path: "/v1/admin/guards/", headers: withCookie(nil), status: 200, body: `[]`},
		{name: "delete answers no content", method: http.MethodDelete, path: "/v1/admin/guards/agent", headers: withCookie(xhr), status: 204},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, header, body := app.doHeaders(testCase.method, testCase.path, "", testCase.headers)
			if status != testCase.status {
				t.Fatalf("status = %d, want %d (%s)", status, testCase.status, body)
			}
			if testCase.body != "" && body != testCase.body {
				t.Fatalf("body = %s, want %s", body, testCase.body)
			}
			for key, want := range testCase.header {
				if got := header.Get(key); got != want {
					t.Fatalf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestBodyOverTheLimitIs413(t *testing.T) {
	app := newTestApp(t, adminAuth(t), &fakeStore{})
	big := `{"guild_id":"` + strings.Repeat("1", bodyLimit) + `"}`
	status, header, body := app.doHeaders(http.MethodPut, "/v1/admin/guards/agent", big, withCookie(xhr))
	if status != http.StatusRequestEntityTooLarge || body != `{"error":"request entity too large"}` {
		t.Fatalf("status = %d body = %s", status, body)
	}
	if header.Get("Connection") != "close" {
		t.Fatalf("Connection = %q", header.Get("Connection"))
	}
}

type panicStore struct{ fakeStore }

func (panicStore) ListGuards(context.Context) ([]store.Guard, error) { panic("boom") }

func TestPanicIsAJSON500(t *testing.T) {
	app := newTestApp(t, adminAuth(t), &panicStore{})
	status, body := app.do(http.MethodGet, "/v1/admin/guards", "", withCookie(nil))
	if status != http.StatusInternalServerError || body != `{"error":"internal server error"}` {
		t.Fatalf("status = %d body = %s", status, body)
	}
}

// eventsTo returns a client that exports to a stand-in Grafana, and a function
// that waits for the request event naming path.
func eventsTo(t *testing.T) (*logclient.Client, func(path string) logclienttest.Event) {
	t.Helper()
	events, received := logclienttest.New(t, "sprinter")
	wait := func(path string) logclienttest.Event {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case event := <-received:
				if event.Message == "HTTP request completed" && event.Payload["path"] == path {
					return event
				}
			case <-timeout:
				t.Fatalf("no request event for %s", path)
			}
		}
	}
	return events, wait
}

// The request log names the operator behind an admin call, and the cause
// behind a failure, as it did when Fiber handlers returned their errors.
func TestRequestEventsCarryActorAndCause(t *testing.T) {
	events, wait := eventsTo(t)
	app := &testApp{t: t, app: New(config.Config{}, &fakeStore{guards: []store.Guard{}},
		adminAuth(t), events, nil)}
	app.do(http.MethodPut, "/v1/admin/guards/agent", `{"guild_id":"nope","role_ids":[]}`, withCookie(xhr))
	event := wait("/v1/admin/guards/agent")
	if event.Level != "warning" || event.Payload["actor"] != "octocat" ||
		event.Payload["error"] != "guild_id must be a Discord id" || event.Payload["status"] != float64(400) {
		t.Fatalf("event = %+v", event)
	}

	down := &testApp{t: t, app: New(config.Config{}, &fakeStore{},
		authAnswering(t, http.StatusInternalServerError), events, nil)}
	down.do(http.MethodGet, "/v1/auth/status", "", withCookie(nil))
	event = wait("/v1/auth/status")
	if event.Level != "error" || event.Payload["error"] != "auth /v1/session returned HTTP 500" {
		t.Fatalf("event = %+v", event)
	}
}

// The page reads /v1/auth/status to choose between the sign-in screen, the
// no-permission notice, and the admin UI, so its fields stay as they were.
func TestAuthStatusBody(t *testing.T) {
	app := newTestApp(t, authAnswering(t, http.StatusUnauthorized), &fakeStore{})
	status, body := app.do(http.MethodGet, "/v1/auth/status", "", nil)
	if want := `{"can_admin":false,"permissions":{"admin":false},"discord_enabled":false}`; status != http.StatusOK || body != want {
		t.Fatalf("anonymous: %d %s, want %s", status, body, want)
	}

	app = newTestApp(t, authAs(t), &fakeStore{})
	status, body = app.do(http.MethodGet, "/v1/auth/status", "", withCookie(nil))
	if status != http.StatusOK || !strings.HasPrefix(body, `{"operator":{"github_id":7,"login":"octocat",`) ||
		!strings.Contains(body, `"can_admin":false`) {
		t.Fatalf("signed in without admin: %d %s", status, body)
	}

	app = newTestApp(t, authAnswering(t, http.StatusInternalServerError), &fakeStore{})
	status, body = app.do(http.MethodGet, "/v1/auth/status", "", withCookie(nil))
	if status != http.StatusServiceUnavailable || body != `{"error":"service unavailable"}` {
		t.Fatalf("Auth down: %d %s", status, body)
	}
}

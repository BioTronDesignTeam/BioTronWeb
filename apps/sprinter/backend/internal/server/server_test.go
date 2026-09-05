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

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/auth"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

type fakeAuth struct {
	status auth.Status
	err    error
}

func (f fakeAuth) Status(context.Context, string) (auth.Status, error) {
	return f.status, f.err
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

func admin() auth.Status {
	return auth.Status{Operator: &auth.Operator{GitHubID: 7, Login: "octocat"}, CanAdmin: true}
}

func newTestApp(t *testing.T, authorizer Authorizer, sprinterStore Store) *fiberApp {
	t.Helper()
	return &fiberApp{t: t, app: New(config.Config{}, sprinterStore, authorizer, &logclient.Client{}, nil)}
}

type fiberApp struct {
	t   *testing.T
	app *fiber.App
}

func (f *fiberApp) do(method, path, body string, headers map[string]string) (int, string) {
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
	response, err := f.app.Test(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(raw)
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
		authorizer Authorizer
		method     string
		headers    map[string]string
		want       int
	}{
		{
			name:       "no cookie is 401",
			authorizer: fakeAuth{},
			method:     http.MethodGet,
			want:       http.StatusUnauthorized,
		},
		{
			name:       "rejected session is 401",
			authorizer: fakeAuth{},
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusUnauthorized,
		},
		{
			name:       "unauthenticated error is 401",
			authorizer: fakeAuth{err: auth.ErrUnauthenticated},
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusUnauthorized,
		},
		{
			name:       "signed in without permission is 403",
			authorizer: fakeAuth{status: auth.Status{Operator: &auth.Operator{Login: "octocat"}}},
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusForbidden,
		},
		{
			name:       "forbidden error is 403",
			authorizer: fakeAuth{err: auth.ErrForbidden},
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusForbidden,
		},
		{
			name:       "transport failure is 503, never a pass",
			authorizer: fakeAuth{err: errors.New("dial tcp: connection refused")},
			method:     http.MethodGet,
			headers:    withCookie(nil),
			want:       http.StatusServiceUnavailable,
		},
		{
			name:       "a write without X-Requested-With is 403",
			authorizer: fakeAuth{status: admin()},
			method:     http.MethodDelete,
			headers:    withCookie(nil),
			want:       http.StatusForbidden,
		},
		{
			name:       "an admin read passes",
			authorizer: fakeAuth{status: admin()},
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
	app := newTestApp(t, fakeAuth{status: admin()}, sprinterStore)

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
	app := newTestApp(t, fakeAuth{status: admin()}, sprinterStore)

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
	app := newTestApp(t, fakeAuth{}, &fakeStore{})
	status, body := app.do(http.MethodGet, "/health", "", nil)
	if status != http.StatusOK || !strings.Contains(body, `"database":"ok"`) {
		t.Fatalf("status = %d body = %s", status, body)
	}
	if !strings.Contains(body, `"discord_connected":false`) {
		t.Fatalf("body = %s", body)
	}

	down := newTestApp(t, fakeAuth{}, &fakeStore{pingErr: errors.New("no connection")})
	status, body = down.do(http.MethodGet, "/health", "", nil)
	if status != http.StatusServiceUnavailable || !strings.Contains(body, `"database":"unavailable"`) {
		t.Fatalf("status = %d body = %s", status, body)
	}
}

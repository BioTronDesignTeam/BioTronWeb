package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

const statusCatalog = `[
  {
    "id": "calendar",
    "name": "BioTron Calendar",
    "description": "Public event calendars and feeds",
    "components": [
      {"id": "calendar-api", "name": "API", "health_url": "http://calendar-api:8080/health"},
      {"id": "calendar-web", "name": "Web", "health_url": "http://calendar-web/"}
    ]
  }
]`

var statusNow = time.Date(2026, 9, 3, 21, 40, 0, 0, time.UTC)

// deniedAuthorizer stands in for a signed-in user without logger/view.
type deniedAuthorizer struct{ decision auth.Decision }

func (a deniedAuthorizer) Authorize(context.Context, string) (auth.Decision, error) {
	return a.decision, nil
}

func statusApp(t *testing.T, store *fakeStore, authorizer auth.Authorizer) *fiber.App {
	t.Helper()
	serviceCatalog, err := catalog.Load(statusCatalog)
	if err != nil {
		t.Fatal(err)
	}
	return New(store, serviceCatalog, authorizer, Options{
		IngestToken:           "secret",
		HealthInterval:        15 * time.Second,
		HealthHistoryInterval: 5 * time.Minute,
		Now:                   func() time.Time { return statusNow },
	})
}

func heartbeats(from, to time.Time, ok bool) []model.HealthPoint {
	points := make([]model.HealthPoint, 0)
	for at := from; at.Before(to); at = at.Add(5 * time.Minute) {
		points = append(points, model.HealthPoint{OK: ok, CheckedAt: at})
	}
	return points
}

func getJSON(t *testing.T, app *fiber.App, path string, into any) *http.Response {
	t.Helper()
	response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), noTestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", path, response.StatusCode)
	}
	if into != nil {
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatal(err)
		}
	}
	return response
}

func TestStatusIsPublicAndHidesOperationalDetail(t *testing.T) {
	store := &fakeStore{
		health: map[string]model.Health{
			"calendar-api": {Service: "calendar-api", OK: true, Detail: "HTTP 200 in 4ms", CheckedAt: statusNow.Add(-10 * time.Second)},
			"calendar-web": {Service: "calendar-web", OK: false, Detail: "dial tcp: lookup calendar-web on 127.0.0.11:53", CheckedAt: statusNow.Add(-10 * time.Second)},
		},
		history: map[string][]model.HealthPoint{
			"calendar-api": heartbeats(statusNow.AddDate(0, 0, -90), statusNow, true),
		},
	}
	// An authorizer that would refuse everything proves the route needs nobody.
	app := statusApp(t, store, deniedAuthorizer{})

	var payload statusResponse
	response := getJSON(t, app, "/v1/status", &payload)
	if got := response.Header.Get("Cache-Control"); got != "public, max-age=30" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if payload.Overall.State != stateDegraded {
		t.Fatalf("overall state = %q, want degraded when one component is down", payload.Overall.State)
	}
	if len(payload.Applications) != 1 || len(payload.Applications[0].Components) != 2 {
		t.Fatalf("applications = %#v", payload.Applications)
	}
	if payload.Applications[0].Components[0].State != stateOperational {
		t.Fatalf("component state = %q", payload.Applications[0].Components[0].State)
	}
	if payload.Applications[0].Components[1].State != stateDown {
		t.Fatalf("component state = %q", payload.Applications[0].Components[1].State)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"detail", "HTTP 200", "127.0.0.11", "health_url", "calendar-api:8080"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("public status leaked %q: %s", leak, raw)
		}
	}
}

func TestStatusReportsTimeWeightedUptimeAndNullWithoutData(t *testing.T) {
	// The API was down for the first two hours of the last day and observed
	// throughout; the web component has never been recorded at all.
	history := heartbeats(statusNow.AddDate(0, 0, -90), statusNow.Add(-24*time.Hour), true)
	history = append(history, heartbeats(statusNow.Add(-24*time.Hour), statusNow.Add(-22*time.Hour), false)...)
	history = append(history, heartbeats(statusNow.Add(-22*time.Hour), statusNow, true)...)
	store := &fakeStore{
		health: map[string]model.Health{
			"calendar-api": {Service: "calendar-api", OK: true, CheckedAt: statusNow.Add(-10 * time.Second)},
		},
		history: map[string][]model.HealthPoint{"calendar-api": history},
	}
	app := statusApp(t, store, auth.AllowAll{})

	var payload statusResponse
	getJSON(t, app, "/v1/status", &payload)
	api := payload.Applications[0].Components[0]
	web := payload.Applications[0].Components[1]

	if api.Uptime24h == nil || *api.Uptime24h != 91.67 {
		t.Fatalf("24h uptime = %v, want 91.67", api.Uptime24h)
	}
	if api.Uptime90d == nil || *api.Uptime90d >= 100 {
		t.Fatalf("90d uptime = %v, want a figure clamped below 100 after an outage", api.Uptime90d)
	}
	if web.Uptime24h != nil || web.Uptime90d != nil {
		t.Fatalf("component with no history = %v/%v, want null", web.Uptime24h, web.Uptime90d)
	}
	if web.State != stateUnknown || web.CheckedAt != nil {
		t.Fatalf("component with no health = %#v", web)
	}
	// The headline aggregates durations, so the never-observed component
	// neither drags it down nor props it up.
	if payload.Overall.Uptime24h == nil || *payload.Overall.Uptime24h != 91.67 {
		t.Fatalf("overall 24h uptime = %v", payload.Overall.Uptime24h)
	}
}

// The page shows one figure and one bar per application until a visitor
// expands it, so each application must carry its own roll-up. It combines the
// components' observed durations: one component down for a quarter of the day
// and another fully up is 87.5% of component-time, not the 75% of the worse one
// or a guess from percentages that hide how long each was watched.
func TestStatusRollsComponentUptimeUpToTheApplication(t *testing.T) {
	dayAgo := statusNow.Add(-24 * time.Hour)
	sixHoursAgo := statusNow.Add(-6 * time.Hour)
	store := &fakeStore{history: map[string][]model.HealthPoint{
		"calendar-api": heartbeats(dayAgo, statusNow, true),
		"calendar-web": append(heartbeats(dayAgo, sixHoursAgo, true), heartbeats(sixHoursAgo, statusNow, false)...),
	}}
	app := statusApp(t, store, deniedAuthorizer{})

	var payload statusResponse
	getJSON(t, app, "/v1/status", &payload)
	application := payload.Applications[0]
	if application.Uptime24h == nil || *application.Uptime24h != 87.5 {
		t.Fatalf("application uptime_24h = %v, want 87.5 (42 up hours of 48 observed)", application.Uptime24h)
	}
	if application.Components[0].Uptime24h == nil || *application.Components[0].Uptime24h != 100 {
		t.Fatalf("api uptime_24h = %v", application.Components[0].Uptime24h)
	}
	if application.Components[1].Uptime24h == nil || *application.Components[1].Uptime24h != 75 {
		t.Fatalf("web uptime_24h = %v", application.Components[1].Uptime24h)
	}
	// One application, so the headline figure is the same roll-up.
	if payload.Overall.Uptime24h == nil || *payload.Overall.Uptime24h != *application.Uptime24h {
		t.Fatalf("overall uptime_24h = %v, want the single application's %v", payload.Overall.Uptime24h, *application.Uptime24h)
	}
}

// An application with no observed component in a window reports null, exactly
// as a component does. Unknown time never becomes a reassuring number.
func TestStatusApplicationWithoutHistoryIsNull(t *testing.T) {
	app := statusApp(t, &fakeStore{}, deniedAuthorizer{})

	var payload statusResponse
	getJSON(t, app, "/v1/status", &payload)
	application := payload.Applications[0]
	if application.Uptime24h != nil || application.Uptime7d != nil || application.Uptime90d != nil {
		t.Fatalf("unobserved application reported uptime %v %v %v, want null", application.Uptime24h, application.Uptime7d, application.Uptime90d)
	}
}

func TestStatusMarksStaleHealthUnknown(t *testing.T) {
	store := &fakeStore{health: map[string]model.Health{
		"calendar-api": {Service: "calendar-api", OK: true, CheckedAt: statusNow.Add(-time.Hour)},
	}}
	app := statusApp(t, store, auth.AllowAll{})

	var payload statusResponse
	getJSON(t, app, "/v1/status", &payload)
	if state := payload.Applications[0].Components[0].State; state != stateUnknown {
		t.Fatalf("state = %q, want unknown for a reading older than three polls", state)
	}
	if payload.Overall.State != stateUnknown {
		t.Fatalf("overall = %q, want unknown when nothing is being observed", payload.Overall.State)
	}
}

func TestHistoryReturnsFixedWidthDailyBuckets(t *testing.T) {
	store := &fakeStore{history: map[string][]model.HealthPoint{
		"calendar-api": heartbeats(statusNow.Add(-48*time.Hour), statusNow, true),
	}}
	app := statusApp(t, store, deniedAuthorizer{})

	var payload historyResponse
	response := getJSON(t, app, "/v1/status/history?days=90", &payload)
	if got := response.Header.Get("Cache-Control"); got != "public, max-age=30" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if payload.Days != 90 || payload.Timezone != "America/Toronto" {
		t.Fatalf("payload = %d days in %q", payload.Days, payload.Timezone)
	}
	if len(payload.Components) != 2 {
		t.Fatalf("components = %d, want one per catalog component", len(payload.Components))
	}
	for _, component := range payload.Components {
		if len(component.Buckets) != 90 {
			t.Fatalf("%s has %d buckets, want exactly 90", component.ID, len(component.Buckets))
		}
	}

	buckets := payload.Components[0].Buckets
	if buckets[0].State != stateUnknown || buckets[0].Uptime != nil {
		t.Fatalf("oldest bucket = %#v, want unknown with a null uptime", buckets[0])
	}
	last := buckets[len(buckets)-1]
	if last.State != stateOperational || last.Uptime == nil || *last.Uptime != 100 {
		t.Fatalf("newest bucket = %#v, want a fully operational day", last)
	}
	if buckets[0].Date >= last.Date {
		t.Fatalf("buckets are not oldest first: %s then %s", buckets[0].Date, last.Date)
	}
	if payload.Components[1].Buckets[0].State != stateUnknown {
		t.Fatalf("component with no history should be unknown throughout")
	}
}

// The daily bar drawn for a collapsed application is the same roll-up as the
// headline figure, computed day by day. A day on which one component was up
// throughout and the other down throughout is a 50% day for the application,
// and a day nobody watched either component stays unknown.
func TestHistoryRollsComponentDaysUpToTheApplication(t *testing.T) {
	twoDaysAgo := statusNow.Add(-48 * time.Hour)
	store := &fakeStore{history: map[string][]model.HealthPoint{
		"calendar-api": heartbeats(twoDaysAgo, statusNow, true),
		"calendar-web": heartbeats(twoDaysAgo, statusNow, false),
	}}
	app := statusApp(t, store, deniedAuthorizer{})

	var payload historyResponse
	getJSON(t, app, "/v1/status/history?days=90", &payload)
	if len(payload.Applications) != 1 {
		t.Fatalf("applications = %d, want one per catalog application", len(payload.Applications))
	}
	application := payload.Applications[0]
	if application.ID != "calendar" || application.Name != "BioTron Calendar" {
		t.Fatalf("application identity = %q %q", application.ID, application.Name)
	}
	if len(application.Buckets) != 90 {
		t.Fatalf("application has %d buckets, want exactly 90", len(application.Buckets))
	}
	if first := application.Buckets[0]; first.State != stateUnknown || first.Uptime != nil {
		t.Fatalf("oldest application bucket = %#v, want unknown with a null uptime", first)
	}
	last := application.Buckets[len(application.Buckets)-1]
	if last.State != stateDown || last.Uptime == nil || *last.Uptime != 50 {
		t.Fatalf("newest application bucket = %#v, want a 50%% day marked down", last)
	}
	if last.Date != payload.Components[0].Buckets[len(payload.Components[0].Buckets)-1].Date {
		t.Fatalf("application and component buckets are not aligned on the same days")
	}
}

func TestHistoryClampsDaysAndNeverEchoesIt(t *testing.T) {
	app := statusApp(t, &fakeStore{}, auth.AllowAll{})
	cases := []struct {
		query string
		want  int
	}{
		{query: "", want: 90},
		{query: "?days=0", want: 1},
		{query: "?days=-4", want: 1},
		{query: "?days=900", want: 90},
		{query: "?days=%3Cscript%3Ealert(1)%3C/script%3E", want: 90},
		{query: "?days=30", want: 30},
	}
	for _, testCase := range cases {
		var payload historyResponse
		getJSON(t, app, "/v1/status/history"+testCase.query, &payload)
		if payload.Days != testCase.want {
			t.Fatalf("days for %q = %d, want %d", testCase.query, payload.Days, testCase.want)
		}
		if len(payload.Components[0].Buckets) != testCase.want {
			t.Fatalf("buckets for %q = %d", testCase.query, len(payload.Components[0].Buckets))
		}
	}
}

func TestStatusRoutesShareOneCachedHistoryFetch(t *testing.T) {
	store := &fakeStore{history: map[string][]model.HealthPoint{
		"calendar-api": heartbeats(statusNow.Add(-24*time.Hour), statusNow, true),
	}}
	app := statusApp(t, store, auth.AllowAll{})

	getJSON(t, app, "/v1/status", nil)
	getJSON(t, app, "/v1/status", nil)
	getJSON(t, app, "/v1/status/history?days=90", nil)
	getJSON(t, app, "/v1/status/history?days=30", nil)

	if store.historyCalls != 1 {
		t.Fatalf("health history queried %d times, want one cached fetch", store.historyCalls)
	}
	// The store collapses runs and marks them continuous using this tolerance,
	// and the uptime walk trusts those marks. If the two ever drift apart, a
	// silence arrives already marked as observed and vanishes without trace.
	if store.historyTolerance != 15*time.Minute {
		t.Fatalf("history tolerance = %s, want the walk's own 3x heartbeat gap", store.historyTolerance)
	}
}

func TestSessionIsPublicAndSeparatesSignedOutFromUnpermitted(t *testing.T) {
	cases := []struct {
		name     string
		decision auth.Decision
	}{
		{name: "signed out", decision: auth.Decision{}},
		{name: "signed in without logger view", decision: auth.Decision{Authenticated: true}},
		{name: "signed in with logger view", decision: auth.Decision{Authenticated: true, Allowed: true}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			app := statusApp(t, &fakeStore{}, deniedAuthorizer{decision: testCase.decision})
			var payload struct {
				Authenticated bool `json:"authenticated"`
				Allowed       bool `json:"allowed"`
			}
			// Always 200: a 401 or 403 here cannot distinguish the first two
			// cases, and that is exactly what caused the sign-in loop.
			getJSON(t, app, "/v1/session", &payload)
			if payload.Authenticated != testCase.decision.Authenticated || payload.Allowed != testCase.decision.Allowed {
				t.Fatalf("session = %#v, want %#v", payload, testCase.decision)
			}
		})
	}
}

func TestLogRoutesStayBehindLoggerView(t *testing.T) {
	app := statusApp(t, &fakeStore{}, deniedAuthorizer{decision: auth.Decision{Authenticated: true}})
	for _, path := range []string{"/v1/apps", "/v1/apps/calendar/logs/recent", "/v1/apps/calendar/logs/history"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), noTestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("GET %s status = %d, want 403 without logger/view", path, response.StatusCode)
		}
	}
}

func TestPublicStatusIsRateLimited(t *testing.T) {
	serviceCatalog, err := catalog.Load(statusCatalog)
	if err != nil {
		t.Fatal(err)
	}
	app := New(&fakeStore{}, serviceCatalog, auth.AllowAll{}, Options{
		IngestToken:     "secret",
		StatusRateLimit: 3,
		Now:             func() time.Time { return statusNow },
	})

	var lastStatus int
	for i := 0; i < 5; i++ {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/status", nil), noTestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		lastStatus = response.StatusCode
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("status after exceeding the limit = %d, want 429", lastStatus)
	}
}

// Behind the edge every request arrives from the same container address, so the
// limiter is only per-client if c.IP() reads the address the edge stamped into
// Cf-Connecting-Ip. Without the trusted-proxy configuration one visitor's burst
// would 429 everybody else.
func TestPublicStatusLimitIsPerClientBehindTheEdge(t *testing.T) {
	serviceCatalog, err := catalog.Load(statusCatalog)
	if err != nil {
		t.Fatal(err)
	}
	app := New(&fakeStore{}, serviceCatalog, auth.AllowAll{}, Options{
		IngestToken:     "secret",
		StatusRateLimit: 1,
		// app.Test dials from 0.0.0.0, standing in for the edge proxy.
		TrustedProxies: []string{"0.0.0.0"},
		Now:            func() time.Time { return statusNow },
	})

	get := func(clientIP string) int {
		request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
		request.Header.Set("Cf-Connecting-Ip", clientIP)
		response, err := app.Test(request, noTestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}

	if got := get("203.0.113.7"); got != http.StatusOK {
		t.Fatalf("first request from 203.0.113.7 = %d, want 200", got)
	}
	if got := get("203.0.113.7"); got != http.StatusTooManyRequests {
		t.Fatalf("second request from 203.0.113.7 = %d, want 429", got)
	}
	if got := get("198.51.100.4"); got != http.StatusOK {
		t.Fatalf("first request from 198.51.100.4 = %d, want 200; the limiter is one global bucket", got)
	}
}

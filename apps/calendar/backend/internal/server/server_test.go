package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	calendarlogic "github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient/authclienttest"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

func TestClampQueryClampsInsteadOfRejecting(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: upcomingDefaultLimit},
		{raw: "not a number", want: upcomingDefaultLimit},
		{raw: "0", want: 1},
		{raw: "-4", want: 1},
		{raw: " 7 ", want: 7},
		{raw: "20", want: upcomingMaxLimit},
		{raw: "5000", want: upcomingMaxLimit},
	}
	for _, test := range tests {
		if got := clampQuery(test.raw, upcomingDefaultLimit, upcomingMaxLimit); got != test.want {
			t.Fatalf("clampQuery(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
	if got := clampQuery("91", upcomingDefaultDays, upcomingMaxDays); got != upcomingMaxDays {
		t.Fatalf("clampQuery days = %d, want %d", got, upcomingMaxDays)
	}
}

// GET /v1/events/upcoming is a published contract for the site and the Sprinter
// bot, so its objects must stay identical to the ones GET /v1/events returns.
func TestUpcomingUsesTheSharedOccurrenceShape(t *testing.T) {
	encoded, err := json.Marshal(model.Occurrence{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	var got []string
	for name := range fields {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{
		"all_day", "description", "ends_at", "location", "modified", "recurrence_id_local",
		"recurring", "scope_id", "scope_kind", "scope_name", "scope_path", "series_id", "series_sequence",
		"starts_at", "timezone", "title", "uid", "url",
	}
	if len(got) != len(want) {
		t.Fatalf("occurrence fields changed: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("occurrence fields changed: %v, want %v", got, want)
		}
	}
}

// The three cases the contract turns on, against a fixture clock so the test
// cannot rot: an occurrence that has ENDED is excluded however recently or
// however long it ran, one IN PROGRESS is included, and one entirely in the
// FUTURE is included.
//
// Asserting only ordering and count would pass while stale events were being
// served, which is exactly how a stale event reached a consumer once.
func TestUpcomingExcludesEndedIncludesInProgressAndFuture(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, location)

	series := []model.EventSeries{
		// ENDED — must never appear.
		newSeries(t, "ended-hours-ago", "Morning standup", "2026-09-03T09:00:00", "2026-09-03T09:30:00"),
		newSeries(t, "ended-a-minute-ago", "Just finished", "2026-09-03T21:00:00", "2026-09-03T22:14:00"),
		newSeries(t, "ended-exactly-now", "Ended on the boundary", "2026-09-03T21:00:00", "2026-09-03T22:15:00"),
		// A long run that began days ago and finished days ago. This is the one
		// that slips through if the lower bound is applied to the start.
		newSeries(t, "long-and-over", "Four-day build", "2026-08-28T09:00:00", "2026-09-01T17:00:00"),
		// A whole day that is already behind us.
		newSeries(t, "all-day-past", "Reading day", "2026-09-02T00:00:00", "2026-09-03T00:00:00"),
		// IN PROGRESS — started before now, ends after now.
		newSeries(t, "in-progress", "Overnight build", "2026-09-03T21:00:00", "2026-09-04T02:00:00"),
		newSeries(t, "long-and-running", "Competition week", "2026-08-31T09:00:00", "2026-09-06T17:00:00"),
		// FUTURE.
		newSeries(t, "starts-exactly-now", "Starting on the boundary", "2026-09-03T22:15:00", "2026-09-03T23:15:00"),
		newSeries(t, "tomorrow", "Controls sync", "2026-09-04T18:00:00", "2026-09-04T19:00:00"),
		// Beyond the window.
		newSeries(t, "beyond-window", "Next term kickoff", "2027-01-11T18:00:00", "2027-01-11T19:00:00"),
	}

	got, err := upcomingOccurrences(series, now, now.AddDate(0, 0, upcomingDefaultDays), upcomingMaxLimit, location)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, occurrence := range got {
		titles = append(titles, occurrence.Title)
		if !occurrence.EndsAt.After(now) {
			t.Fatalf("%q ended at %s, before the %s request, and must not be upcoming",
				occurrence.Title, occurrence.EndsAt, now)
		}
	}
	want := []string{"Competition week", "Overnight build", "Starting on the boundary", "Controls sync"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("upcoming returned %v, want %v", titles, want)
	}

	// Truncation keeps the earliest, and the limit is honoured after expansion.
	limited, err := upcomingOccurrences(series, now, now.AddDate(0, 0, upcomingDefaultDays), 2, location)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Title != "Competition week" || limited[1].Title != "Overnight build" {
		t.Fatalf("truncation kept the wrong occurrences: %+v", limited)
	}
}

// A weekly series must contribute only the instances inside the window, so the
// filter has to run after expansion rather than keeping or dropping the series
// as a whole.
func TestUpcomingFiltersInstancesNotWholeSeries(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, location)
	weekly := newSeries(t, "weekly", "Controls sync", "2026-08-14T18:00:00", "2026-08-14T19:00:00")
	until, err := calendarlogic.ParseLocalDate("2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	weekly.RecurrenceUntil = &until

	got, err := upcomingOccurrences([]model.EventSeries{weekly}, now, now.AddDate(0, 0, upcomingDefaultDays), upcomingMaxLimit, location)
	if err != nil {
		t.Fatal(err)
	}
	var days []string
	for _, occurrence := range got {
		days = append(days, occurrence.StartsAt.Format("2006-01-02"))
	}
	// 14, 21 and 28 August are behind us; 4, 11, 18 and 25 September are not.
	if !reflect.DeepEqual(days, []string{"2026-09-04", "2026-09-11", "2026-09-18", "2026-09-25"}) {
		t.Fatalf("expected only the instances after the request instant, got %v", days)
	}
}

// An empty result must marshal as [] rather than null, because a consumer
// iterating the response should not have to special-case a missing array.
func TestUpcomingReturnsAnEmptyArrayNotNull(t *testing.T) {
	location, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2026, 9, 3, 22, 15, 0, 0, location)
	got, err := upcomingOccurrences(nil, now, now.AddDate(0, 0, 1), 5, location)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("empty upcoming marshalled as %s", encoded)
	}
}

func TestETagMatchesWeakAndListedValidators(t *testing.T) {
	etag := `"abc123"`
	for _, header := range []string{`"abc123"`, `W/"abc123"`, `"other", "abc123"`, `*`} {
		if !etagMatches(header, etag) {
			t.Fatalf("If-None-Match %q should have matched %s", header, etag)
		}
	}
	for _, header := range []string{`"other"`, `W/"other", "another"`, ``} {
		if etagMatches(header, etag) {
			t.Fatalf("If-None-Match %q should not have matched %s", header, etag)
		}
	}
}

func newSeries(t *testing.T, id, title, startsAt, endsAt string) model.EventSeries {
	t.Helper()
	start, err := calendarlogic.ParseLocalDateTime(startsAt)
	if err != nil {
		t.Fatal(err)
	}
	end, err := calendarlogic.ParseLocalDateTime(endsAt)
	if err != nil {
		t.Fatal(err)
	}
	return model.EventSeries{
		ID: id, UID: id + "@biotron.ca", ScopeID: "scope", ScopeName: "Teamwide", ScopeKind: model.ScopeTeam,
		State: model.EventPublished, Title: title, StartsAtLocal: start, EndsAtLocal: end,
		Timezone: "America/Toronto",
	}
}

var ada = authclient.Operator{GitHubID: 4242, Login: "ada", Name: "Ada Lovelace"}

// editorAuth is a stand-in Auth that signs every cookie in as Ada, who holds
// calendar/write.
func editorAuth(t *testing.T) *authclienttest.Auth {
	t.Helper()
	stub := authclienttest.New(t)
	stub.SignIn(ada, PermissionWrite)
	return stub
}

// waitForEvent picks one message out of the stream, because every request also
// produces the middleware's completion event.
func waitForEvent(t *testing.T, sent <-chan logclienttest.Event, message string) logclienttest.Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-sent:
			if event.Message == message {
				return event
			}
		case <-deadline:
			t.Fatalf("no %q event arrived", message)
		}
	}
}

// An admin event is only useful if it names who caused it, so this drives the
// whole chain: the gate resolves the operator from Auth, parks it on the
// request, and adminEvent puts it in the payload.
func TestAdminEventNamesTheOperator(t *testing.T) {
	events, sent := logclienttest.New(t, "calendar-api")
	handler := &Handler{events: events}
	authz := withCalendarErrors(editorAuth(t).Client(AppID), events)

	app := authz.Require(PermissionWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.adminEvent(r, "Scope created", map[string]any{
			"scope_id": "scope-1", "kind": model.ScopeProject, "name": "Drivetrain", "parent_id": "root",
		})
		w.WriteHeader(http.StatusCreated)
	}))

	response := serveTest(app, editorRequest(http.MethodPost, "/v1/admin/scopes"))
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("editor request returned %d", response.StatusCode)
	}

	event := waitForEvent(t, sent, "Scope created")
	if event.Level != logclient.Info {
		t.Fatalf("Scope created was sent at %q", event.Level)
	}
	if got := event.Payload["actor_login"]; got != "ada" {
		t.Fatalf("actor_login = %v", got)
	}
	if got := event.Payload["actor_id"]; got != float64(4242) {
		t.Fatalf("actor_id = %v", got)
	}
	if got := event.Payload["scope_id"]; got != "scope-1" {
		t.Fatalf("scope_id = %v", got)
	}
}

// Moving the gate to go/authclient must not change who is let in or what the
// admin page is told, so every branch is pinned with its body.
func TestWriteGateKeepsItsStatusMapping(t *testing.T) {
	events, sent := logclienttest.New(t, "calendar-api")
	tests := []struct {
		name    string
		arrange func(*authclienttest.Auth)
		cookie  string
		omitXHR bool
		want    int
		body    string
	}{
		{name: "editor", arrange: func(a *authclienttest.Auth) { a.SignIn(ada, PermissionWrite) }, cookie: "session=abc", want: http.StatusCreated},
		{name: "no cookie", arrange: func(a *authclienttest.Auth) { a.SignIn(ada, PermissionWrite) }, want: http.StatusUnauthorized, body: `{"error":"unauthorized"}`},
		{name: "session rejected", arrange: func(a *authclienttest.Auth) { a.Answer(http.StatusUnauthorized) }, cookie: "session=abc", want: http.StatusUnauthorized, body: `{"error":"unauthorized"}`},
		{name: "a guest of another tool", arrange: func(a *authclienttest.Auth) { a.Answer(http.StatusForbidden) }, cookie: "session=abc", want: http.StatusUnauthorized, body: `{"error":"unauthorized"}`},
		{name: "permission missing", arrange: func(a *authclienttest.Auth) { a.SignIn(ada) }, cookie: "session=abc", want: http.StatusForbidden, body: `{"error":"forbidden"}`},
		{name: "auth broken", arrange: func(a *authclienttest.Auth) { a.Answer(http.StatusInternalServerError) }, cookie: "session=abc", want: http.StatusServiceUnavailable, body: `{"error":"service unavailable"}`},
		{name: "no X-Requested-With", arrange: func(a *authclienttest.Auth) { a.SignIn(ada, PermissionWrite) }, cookie: "session=abc", omitXHR: true, want: http.StatusForbidden, body: `{"error":"forbidden"}`},
	}

	for _, test := range tests {
		stub := authclienttest.New(t)
		test.arrange(stub)
		app := withCalendarErrors(stub.Client(AppID), events).Require(PermissionWrite)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		request := editorRequest(http.MethodPost, "/v1/admin/scopes")
		request.Header.Del("Cookie")
		if test.cookie != "" {
			request.Header.Set("Cookie", test.cookie)
		}
		if test.omitXHR {
			request.Header.Del("X-Requested-With")
		}
		response := serveTest(app, request)
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != test.want || string(body) != test.body {
			t.Fatalf("%s returned %d %s, want %d %s", test.name, response.StatusCode, body, test.want, test.body)
		}
	}

	// An unreachable Auth is the one refusal an operator cannot diagnose from
	// the response, so it has to be exported as well as written to stdout.
	event := waitForEvent(t, sent, "Authorization service unavailable")
	if event.Level != logclient.Error {
		t.Fatalf("Authorization service unavailable was sent at %q", event.Level)
	}
	if _, ok := event.Payload["error"]; !ok {
		t.Fatalf("Authorization service unavailable carried no error: %v", event.Payload)
	}
}

// The public routes must never ask Auth, even when the browser sends a
// session cookie: feeds and the site read them without credentials.
func TestPublicRoutesNeverAskAuth(t *testing.T) {
	stub := editorAuth(t)
	app := New(config.Config{FrontendURL: "http://localhost:5176"}, nil, stub.Client(AppID), logclient.NewFromEnv("calendar-api"), time.UTC)
	for _, target := range []string{"/v1/events?from=nope", "/v1/events/upcoming?from=nope", "/v1/feeds/scopes/nope"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Cookie", "session=abc")
		response := serveTest(app, request)
		response.Body.Close()
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			t.Fatalf("GET %s returned %d", target, response.StatusCode)
		}
	}
	if stub.Calls() != 0 {
		t.Fatalf("public routes asked Auth %d times", stub.Calls())
	}
}

func editorRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	request.Header.Set("Cookie", "session=abc")
	return request
}

// A refused request must be logged as the status the client got. The
// middleware runs the app's error handler itself, so store.ErrNotFound and
// store.ErrConflict are logged as 404 and 409 rather than guessed at from the
// error's type. This drives the real app so the middleware order is part of
// what is pinned.
func TestRefusedRequestIsLoggedWithTheStatusTheClientGot(t *testing.T) {
	events, sent := logclienttest.New(t, "calendar-api")
	app := New(
		config.Config{FrontendURL: "http://localhost:5176"},
		nil, authclient.New("", AppID), events, time.UTC,
	)

	response := serveTest(app, httptest.NewRequest(http.MethodGet, "/v1/nope", nil))
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route returned %d", response.StatusCode)
	}

	event := waitForEvent(t, sent, "HTTP request completed")
	if event.Level != logclient.Warning {
		t.Fatalf("a 404 was logged at %q", event.Level)
	}
	if got := event.Payload["status"]; got != float64(http.StatusNotFound) {
		t.Fatalf("status = %v, want 404", got)
	}
	if got := event.Payload["path"]; got != "/v1/nope" {
		t.Fatalf("path = %v", got)
	}
}

// serveTest runs one request through handler and returns the response the
// client would have received.
func serveTest(handler http.Handler, request *http.Request) *http.Response {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Result()
}

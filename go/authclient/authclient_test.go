package authclient_test

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient/authclienttest"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

const cookie = "oauth_session=abc"

var ada = authclient.Operator{GitHubID: 4242, Login: "ada", Name: "Ada Lovelace"}

// silenceLog keeps the deliberate fail-closed log lines out of test output.
func silenceLog(t *testing.T) {
	t.Helper()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}

// sink collects request events, which httplog sends synchronously.
type sink struct {
	mu       sync.Mutex
	payloads []map[string]any
}

func (s *sink) LogAsync(_ logclient.Level, _ string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields, _ := payload.(map[string]any)
	s.payloads = append(s.payloads, fields)
}

func (s *sink) last() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payloads) == 0 {
		return nil
	}
	return s.payloads[len(s.payloads)-1]
}

type response struct {
	status int
	header http.Header
	body   string
}

// gated serves one request through Require(permission), inside httplog, and
// reports what the client got.
func gated(t *testing.T, client *authclient.Client, permission, method, cookieHeader string, headers map[string]string) response {
	t.Helper()
	inner := client.Require(permission)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	request := httptest.NewRequest(method, "/v1/probe", nil)
	if cookieHeader != "" {
		request.Header.Set("Cookie", cookieHeader)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	httplog.New(&sink{}, httplog.Options{})(inner).ServeHTTP(recorder, request)
	return response{recorder.Code, recorder.Header(), recorder.Body.String()}
}

func TestRequireMapsEveryOutcome(t *testing.T) {
	silenceLog(t)
	xhr := map[string]string{"X-Requested-With": "XMLHttpRequest"}
	for _, tc := range []struct {
		name    string
		arrange func(*authclienttest.Auth)
		method  string
		cookie  string
		headers map[string]string
		want    int
		asks    bool
	}{
		{name: "no cookie is 401 without asking Auth", method: http.MethodGet, want: http.StatusUnauthorized},
		{name: "a refused session is 401", method: http.MethodGet, cookie: cookie, want: http.StatusUnauthorized, asks: true},
		{
			name:    "a guest of another tool is 401",
			arrange: func(a *authclienttest.Auth) { a.Answer(http.StatusForbidden) },
			method:  http.MethodGet, cookie: cookie, want: http.StatusUnauthorized, asks: true,
		},
		{
			name:    "signed in without the permission is 403",
			arrange: func(a *authclienttest.Auth) { a.SignIn(ada, "other") },
			method:  http.MethodGet, cookie: cookie, want: http.StatusForbidden, asks: true,
		},
		{
			name:    "signed in with the permission passes",
			arrange: func(a *authclienttest.Auth) { a.SignIn(ada, "write") },
			method:  http.MethodGet, cookie: cookie, want: http.StatusOK, asks: true,
		},
		{
			name:    "a write without X-Requested-With is 403 without asking Auth",
			arrange: func(a *authclienttest.Auth) { a.SignIn(ada, "write") },
			method:  http.MethodPost, cookie: cookie, want: http.StatusForbidden,
		},
		{
			name:    "a write with X-Requested-With passes",
			arrange: func(a *authclienttest.Auth) { a.SignIn(ada, "write") },
			method:  http.MethodDelete, cookie: cookie, headers: xhr, want: http.StatusOK, asks: true,
		},
		{
			name:    "a broken Auth is 503, never a pass",
			arrange: func(a *authclienttest.Auth) { a.Answer(http.StatusInternalServerError) },
			method:  http.MethodGet, cookie: cookie, want: http.StatusServiceUnavailable, asks: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := authclienttest.New(t)
			if tc.arrange != nil {
				tc.arrange(auth)
			}
			got := gated(t, auth.Client("calendar"), "write", tc.method, tc.cookie, tc.headers)
			if got.status != tc.want {
				t.Fatalf("status = %d, want %d (%s)", got.status, tc.want, got.body)
			}
			if asked := auth.Calls() > 0; asked != tc.asks {
				t.Fatalf("asked Auth = %v, want %v", asked, tc.asks)
			}
		})
	}
}

// Auth is asked about this tool and this permission, with the caller's cookie.
func TestRequireAsksAboutTheToolAndPermission(t *testing.T) {
	auth := authclienttest.New(t)
	auth.SignIn(ada, "commands")
	gated(t, auth.Client("exo-gui"), "commands", http.MethodGet, cookie, nil)
	query := auth.LastQuery()
	if query.Get("app") != "exo-gui" || strings.Join(query["permission"], ",") != "commands" {
		t.Fatalf("query = %v", query)
	}
}

// The handler behind the gate learns who passed, and so does the request log.
func TestRequireHandsOnTheOperator(t *testing.T) {
	auth := authclienttest.New(t)
	auth.SignIn(ada, "write")
	client := auth.Client("calendar")

	events := &sink{}
	var operator *authclient.Operator
	var session authclient.Session
	handler := httplog.New(events, httplog.Options{})(client.Require("write")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		operator = authclient.OperatorFrom(r)
		session, _ = authclient.SessionFrom(r)
	})))
	request := httptest.NewRequest(http.MethodGet, "/v1/probe", nil)
	request.Header.Set("Cookie", cookie)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if operator == nil || operator.Login != "ada" || operator.GitHubID != 4242 {
		t.Fatalf("OperatorFrom = %+v", operator)
	}
	if !session.Can("write") || session.Can("admin") {
		t.Fatalf("session = %+v", session)
	}
	if got := events.last()["actor"]; got != "ada" {
		t.Fatalf("request event actor = %v", got)
	}
	if authclient.OperatorFrom(httptest.NewRequest(http.MethodGet, "/", nil)) != nil {
		t.Fatal("OperatorFrom found an operator on a request that never passed the gate")
	}
}

// An Auth that cannot be reached, or is not configured, fails closed and says
// so as an Error event.
func TestRequireFailsClosed(t *testing.T) {
	silenceLog(t)
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := dead.URL
	dead.Close()

	events, sent := logclienttest.New(t, "authclient-test")
	for name, client := range map[string]*authclient.Client{
		"unreachable":    authclient.New(address, "exo-gui", authclient.WithEvents(events)),
		"not configured": authclient.New("", "exo-gui", authclient.WithEvents(events)),
	} {
		got := gated(t, client, "live", http.MethodGet, cookie, nil)
		if got.status != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503", name, got.status)
		}
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-sent:
			if event.Message != "Authorization service unavailable" {
				continue
			}
			if event.Level != logclient.Error || event.Payload["permission"] != "live" || event.Payload["error"] == nil {
				t.Fatalf("event = %+v", event)
			}
			return
		case <-deadline:
			t.Fatal("no Authorization service unavailable event arrived")
		}
	}
}

// Exo's frontend was written against these bodies, so they are the default.
func TestDefaultRefusalBodies(t *testing.T) {
	silenceLog(t)
	auth := authclienttest.New(t)
	auth.SignIn(ada)
	for _, tc := range []struct {
		name   string
		client *authclient.Client
		method string
		cookie string
		status int
		body   string
	}{
		{"no session", auth.Client("exo-gui"), http.MethodGet, "", http.StatusUnauthorized, `{"error":"authentication required"}`},
		{"no permission", auth.Client("exo-gui"), http.MethodGet, cookie, http.StatusForbidden, `{"error":"permission required","permission":"commands"}`},
		{"no XHR header", auth.Client("exo-gui"), http.MethodPost, cookie, http.StatusForbidden, `{"error":"X-Requested-With header required"}`},
		{"not configured", authclient.New("", "exo-gui"), http.MethodGet, cookie, http.StatusServiceUnavailable, `{"error":"authorization service unavailable"}`},
	} {
		got := gated(t, tc.client, "commands", tc.method, tc.cookie, nil)
		if got.status != tc.status || got.body != tc.body {
			t.Errorf("%s: %d %s, want %d %s", tc.name, got.status, got.body, tc.status, tc.body)
		}
		if ct := got.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", tc.name, ct)
		}
	}
}

// A tool keeps its own error format.
func TestWithErrorWriter(t *testing.T) {
	auth := authclienttest.New(t)
	var refusal authclient.Refusal
	client := auth.Client("calendar").With(authclient.WithErrorWriter(func(w http.ResponseWriter, _ *http.Request, r authclient.Refusal) {
		refusal = r
		w.WriteHeader(r.Status)
		_, _ = io.WriteString(w, strings.ToLower(http.StatusText(r.Status)))
	}))
	got := gated(t, client, "write", http.MethodGet, cookie, nil)
	if got.status != http.StatusUnauthorized || got.body != "unauthorized" {
		t.Fatalf("%d %q", got.status, got.body)
	}
	if !errors.Is(refusal.Err, authclient.ErrUnauthenticated) || refusal.Permission != "write" {
		t.Fatalf("refusal = %+v", refusal)
	}
}

func TestLookup(t *testing.T) {
	auth := authclienttest.New(t)
	auth.SignIn(ada, "live", "historical")
	session, err := auth.Client("exo-gui").Lookup(context.Background(), cookie, "live", "commands")
	if err != nil {
		t.Fatal(err)
	}
	if !session.Authenticated() || !session.Can("live") || session.Can("commands") {
		t.Fatalf("session = %+v", session)
	}
	// Only what was asked has a verdict, even if Auth were to say more.
	if session.Can("historical") || len(session.Permissions) != 2 {
		t.Fatalf("permissions = %v", session.Permissions)
	}
	if got := auth.LastQuery()["permission"]; strings.Join(got, ",") != "live,commands" {
		t.Fatalf("permissions asked = %v", got)
	}
}

// An Auth without /v1/session is asked the old way, and a session that ends
// between its two calls is signed out, not an outage.
func TestLookupFallsBackToMeAndCheck(t *testing.T) {
	var checkStatus = http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/me":
			if r.URL.Query().Get("app") != "sprinter" {
				t.Errorf("/auth/me app = %q", r.URL.Query().Get("app"))
			}
			_, _ = io.WriteString(w, `{"github_id":7,"login":"octocat"}`)
		case "/v1/check":
			w.WriteHeader(checkStatus)
			_, _ = io.WriteString(w, `{"allowed":`+map[bool]string{true: "true", false: "false"}[r.URL.Query().Get("permission") == "admin"]+`}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	client := authclient.New(server.URL, "sprinter")

	session, err := client.Lookup(context.Background(), cookie, "admin", "other")
	if err != nil {
		t.Fatal(err)
	}
	if session.Operator == nil || session.Operator.Login != "octocat" || !session.Can("admin") || session.Can("other") {
		t.Fatalf("session = %+v", session)
	}

	checkStatus = http.StatusUnauthorized
	session, err = client.Lookup(context.Background(), cookie, "admin")
	if err != nil || session.Authenticated() {
		t.Fatalf("session ended between calls: %+v, %v", session, err)
	}

	checkStatus = http.StatusBadGateway
	if _, err := client.Lookup(context.Background(), cookie, "admin"); err == nil {
		t.Fatal("a broken /v1/check was not an error")
	}
}

func TestLookupRefusesAnAnswerWithoutAnOperator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"permissions":{"write":true}}`)
	}))
	t.Cleanup(server.Close)
	if _, err := authclient.New(server.URL, "calendar").Lookup(context.Background(), cookie, "write"); err == nil {
		t.Fatal("an answer without an operator was read as a session")
	}
}

func serveStatus(t *testing.T, handler http.Handler, cookieHeader string) response {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/v1/auth/status", nil)
	if cookieHeader != "" {
		request.Header.Set("Cookie", cookieHeader)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return response{recorder.Code, recorder.Header(), recorder.Body.String()}
}

func TestStatusHandler(t *testing.T) {
	silenceLog(t)
	auth := authclienttest.New(t)
	handler := auth.Client("sprinter").StatusHandler(
		[]authclient.StatusField{{Permission: "admin", Field: "can_admin"}},
		func(*http.Request) map[string]any { return map[string]any{"discord_enabled": true} },
	)

	got := serveStatus(t, handler, "")
	if want := `{"can_admin":false,"permissions":{"admin":false},"discord_enabled":true}`; got.status != http.StatusOK || got.body != want {
		t.Fatalf("anonymous: %d %s, want %s", got.status, got.body, want)
	}

	auth.SignIn(authclient.Operator{GitHubID: 7, Login: "octocat"}, "admin")
	got = serveStatus(t, handler, cookie)
	want := `{"operator":{"github_id":7,"login":"octocat","name":"","avatar_url":"","is_superuser":false,"is_manager":false,"is_staff":false,"is_guest":false,"guest_app_id":""},` +
		`"can_admin":true,"permissions":{"admin":true},"discord_enabled":true}`
	if got.status != http.StatusOK || got.body != want {
		t.Fatalf("signed in: %d %s, want %s", got.status, got.body, want)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}

	auth.Answer(http.StatusInternalServerError)
	if got := serveStatus(t, handler, cookie); got.status != http.StatusServiceUnavailable {
		t.Fatalf("broken Auth: %d %s", got.status, got.body)
	}
}

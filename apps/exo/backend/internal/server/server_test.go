package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient/authclienttest"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

const frontend = "http://localhost:5174"

type event struct {
	level   logclient.Level
	payload map[string]any
}

// recorder stands in for the event client.
type recorder struct {
	mu     sync.Mutex
	events []event
}

func (r *recorder) LogAsync(level logclient.Level, _ string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fields, _ := payload.(map[string]any)
	r.events = append(r.events, event{level, fields})
}

func (r *recorder) take() []event {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := r.events
	r.events = nil
	return events
}

// testHandler is New with extra routes mounted inside the same middleware.
func testHandler(sink *recorder, authz *authclient.Client, extra func(*http.ServeMux, gated)) http.Handler {
	if authz == nil {
		authz = authclient.New("http://oauth-manager:8080", auth.AppID)
	}
	return newHandler(frontend, []string{"127.0.0.1"}, sink, authz, extra)
}

type response struct {
	status int
	header http.Header
	body   string
}

func do(t *testing.T, handler http.Handler, method, path string, headers map[string]string) response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return response{rec.Code, rec.Header(), rec.Body.String()}
}

func (r response) expect(t *testing.T, status int, body string, headers map[string]string) {
	t.Helper()
	if r.status != status || r.body != body {
		t.Errorf("got %d %q, want %d %q", r.status, r.body, status, body)
	}
	for key, want := range headers {
		if got := strings.Join(r.header.Values(key), ", "); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestHealth(t *testing.T) {
	sink := &recorder{}
	handler := testHandler(sink, nil, nil)

	do(t, handler, http.MethodGet, "/health", nil).expect(t, http.StatusOK, `{"status":"ok"}`, map[string]string{
		"Content-Type":   "application/json; charset=utf-8",
		"Content-Length": "15",
		"Vary":           "Origin",
	})
	// HEAD over a real connection, where net/http drops the body but keeps
	// the length, as Fiber did.
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	headRequest, err := http.NewRequestWithContext(t.Context(), http.MethodHead, srv.URL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err := http.DefaultClient.Do(headRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = head.Body.Close()
	if head.StatusCode != http.StatusOK || head.ContentLength != 15 {
		t.Fatalf("HEAD /health = %d with length %d, want 200 with 15", head.StatusCode, head.ContentLength)
	}

	if events := sink.take(); len(events) != 0 {
		t.Fatalf("/health must never be logged; got %+v", events)
	}
}

// Requests no route answers keep Fiber's plain-text wording, and a wrong
// method still names the right ones.
func TestUnmatchedRequestsAnswerLikeFiber(t *testing.T) {
	sink := &recorder{}
	handler := testHandler(sink, nil, nil)

	do(t, handler, http.MethodGet, "/nope", nil).expect(t, http.StatusNotFound, "Not Found", map[string]string{
		"Content-Type": "text/plain; charset=utf-8",
		"Vary":         "Origin",
	})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		do(t, handler, method, "/health", nil).expect(t, http.StatusMethodNotAllowed, "Method Not Allowed", map[string]string{
			"Content-Type": "text/plain; charset=utf-8",
			"Allow":        "GET, HEAD",
		})
	}

	// /health is never logged, whatever the method; the 404 is.
	events := sink.take()
	if len(events) != 1 {
		t.Fatalf("events = %+v, want only the 404", events)
	}
	if got := events[0]; got.level != logclient.Warning || got.payload["status"] != 404 || got.payload["error"] != "Not Found" {
		t.Fatalf("404 event = %+v", got)
	}
}

func TestCORS(t *testing.T) {
	handler := testHandler(&recorder{}, nil, nil)
	preflight := map[string]string{
		"Origin":                         frontend,
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "content-type",
	}
	preflightVary := "Access-Control-Request-Method, Access-Control-Request-Headers, Origin"

	// A preflight is answered for any path, routed or not.
	for _, path := range []string{"/health", "/v1/not-yet"} {
		do(t, handler, http.MethodOptions, path, preflight).expect(t, http.StatusNoContent, "", map[string]string{
			"Access-Control-Allow-Origin":      frontend,
			"Access-Control-Allow-Credentials": "true",
			"Access-Control-Allow-Methods":     "GET, POST, OPTIONS",
			"Access-Control-Allow-Headers":     "Content-Type, X-Requested-With",
			"Vary":                             preflightVary,
		})
	}

	// Another origin's preflight gets no Allow-Origin, which is the refusal.
	preflight["Origin"] = "http://evil.example"
	do(t, handler, http.MethodOptions, "/health", preflight).expect(t, http.StatusNoContent, "", map[string]string{
		"Access-Control-Allow-Origin":      "",
		"Access-Control-Allow-Credentials": "",
		"Vary":                             preflightVary,
	})

	// Simple requests: the frontend, matched without regard to case and
	// echoed as sent; anyone else gets the response without the grant.
	do(t, handler, http.MethodGet, "/health", map[string]string{"Origin": "http://LOCALHOST:5174"}).
		expect(t, http.StatusOK, `{"status":"ok"}`, map[string]string{
			"Access-Control-Allow-Origin":      "http://LOCALHOST:5174",
			"Access-Control-Allow-Credentials": "true",
			"Vary":                             "Origin",
		})
	do(t, handler, http.MethodGet, "/health", map[string]string{"Origin": "http://evil.example"}).
		expect(t, http.StatusOK, `{"status":"ok"}`, map[string]string{
			"Access-Control-Allow-Origin": "",
			"Vary":                        "Origin",
		})

	// An OPTIONS that is not a preflight falls through to the router.
	do(t, handler, http.MethodOptions, "/health", map[string]string{"Origin": frontend}).
		expect(t, http.StatusMethodNotAllowed, "Method Not Allowed", map[string]string{
			"Access-Control-Allow-Origin": "",
			"Vary":                        "Origin",
		})
}

// A panicking handler answers 500 with the panic as text, as Fiber's recover
// did, and the event client hears about it as an error.
func TestPanicIsAnswered500AndLogged(t *testing.T) {
	sink := &recorder{}
	handler := testHandler(sink, nil, func(mux *http.ServeMux, _ gated) {
		mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	})
	silenceStderr(t)

	do(t, handler, http.MethodGet, "/panic", nil).expect(t, http.StatusInternalServerError, "boom", map[string]string{
		"Content-Type": "text/plain; charset=utf-8",
	})
	events := sink.take()
	if len(events) != 1 || events[0].level != logclient.Error || events[0].payload["status"] != 500 || events[0].payload["error"] != "boom" {
		t.Fatalf("events = %+v", events)
	}
}

func TestOversizedBodyIsRefused(t *testing.T) {
	var read int
	handler := testHandler(&recorder{}, nil, func(mux *http.ServeMux, _ gated) {
		mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			read = len(body)
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("a", bodyLimit+1)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || rec.Body.String() != "Request Entity Too Large" {
		t.Fatalf("declared oversize body: %d %q", rec.Code, rec.Body.String())
	}
	if read != 0 {
		t.Fatal("the handler ran for a body over the limit")
	}

	// A body that hides its length is cut off at the limit instead.
	req = httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("a", bodyLimit+1)))
	req.ContentLength = -1
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if read != bodyLimit {
		t.Fatalf("handler read %d bytes of an undeclared body, want the %d-byte limit", read, bodyLimit)
	}
}

// gatedHandler mounts a probe route under each gated prefix, the way the real
// routes will be, and answers sessions from a stand-in Auth.
func gatedHandler(t *testing.T, sink *recorder) (http.Handler, *authclienttest.Auth) {
	t.Helper()
	stub := authclienttest.New(t)
	probe := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]any{"ok": true})
	}
	handler := testHandler(sink, stub.Client(auth.AppID), func(_ *http.ServeMux, routes gated) {
		routes.live.HandleFunc("GET /v1/live/probe", probe)
		routes.historical.HandleFunc("GET /v1/historical/probe", probe)
		routes.commands.HandleFunc("POST /v1/commands/probe", probe)
	})
	return handler, stub
}

// Each prefix needs its own permission. A daily guest opens Live and
// Historical and never Commands, and a member without an Exo GUI permission
// opens nothing.
func TestEachPrefixNeedsItsPermission(t *testing.T) {
	handler, stub := gatedHandler(t, &recorder{})
	signedIn := map[string]string{"Cookie": "oauth_session=abc", "X-Requested-With": "XMLHttpRequest"}
	guest := authclient.Operator{Login: "guest", IsGuest: true, GuestAppID: auth.AppID}
	member := authclient.Operator{GitHubID: 7, Login: "octocat"}

	for _, tc := range []struct {
		name    string
		arrange func()
		headers map[string]string
		want    [3]int // live, historical, commands
	}{
		{"anonymous", func() {}, map[string]string{"X-Requested-With": "XMLHttpRequest"}, [3]int{401, 401, 401}},
		{"daily guest", func() { stub.SignIn(guest, auth.PermissionLive, auth.PermissionHistorical) }, signedIn, [3]int{200, 200, 403}},
		{"member without a grant", func() { stub.SignIn(member) }, signedIn, [3]int{403, 403, 403}},
		{"member with every grant", func() {
			stub.SignIn(member, auth.PermissionLive, auth.PermissionHistorical, auth.PermissionCommands)
		}, signedIn, [3]int{200, 200, 200}},
	} {
		tc.arrange()
		for i, route := range []struct{ method, path string }{
			{http.MethodGet, "/v1/live/probe"},
			{http.MethodGet, "/v1/historical/probe"},
			{http.MethodPost, "/v1/commands/probe"},
		} {
			if got := do(t, handler, route.method, route.path, tc.headers); got.status != tc.want[i] {
				t.Errorf("%s: %s %s = %d %s, want %d", tc.name, route.method, route.path, got.status, got.body, tc.want[i])
			}
		}
	}

	// A command without the CSRF header is refused before Auth is asked.
	calls := stub.Calls()
	do(t, handler, http.MethodPost, "/v1/commands/probe", map[string]string{"Cookie": "oauth_session=abc"}).
		expect(t, http.StatusForbidden, `{"error":"X-Requested-With header required"}`, nil)
	if stub.Calls() != calls {
		t.Fatal("Auth was asked about a command without X-Requested-With")
	}

	// An unknown path under a prefix asks for a session like a real one.
	stub.SignIn(member, auth.PermissionLive)
	do(t, handler, http.MethodGet, "/v1/live/nope", nil).expect(t, http.StatusUnauthorized, `{"error":"authentication required"}`, nil)
	do(t, handler, http.MethodGet, "/v1/live/nope", signedIn).expect(t, http.StatusNotFound, "Not Found", nil)
}

// The gate's answers pass through the stack unchanged, and an Auth outage
// is logged as an error carrying its cause.
func TestGatedRouteThroughTheStack(t *testing.T) {
	sink := &recorder{}
	handler, stub := gatedHandler(t, sink)
	silenceLog(t)

	stub.SignIn(authclient.Operator{Login: "octocat"}, auth.PermissionLive)
	do(t, handler, http.MethodGet, "/v1/live/probe", map[string]string{"Cookie": "s=ok", "Origin": frontend}).
		expect(t, http.StatusOK, `{"ok":true}`, map[string]string{"Access-Control-Allow-Origin": frontend})
	do(t, handler, http.MethodGet, "/v1/live/probe", nil).
		expect(t, http.StatusUnauthorized, `{"error":"authentication required"}`, nil)
	stub.Answer(http.StatusInternalServerError)
	do(t, handler, http.MethodGet, "/v1/live/probe", map[string]string{"Cookie": "s=broken"}).
		expect(t, http.StatusServiceUnavailable, `{"error":"authorization service unavailable"}`, nil)

	events := sink.take()
	if len(events) != 3 {
		t.Fatalf("events = %+v", events)
	}
	if allowed := events[0]; allowed.payload["actor"] != "octocat" {
		t.Fatalf("allowed event = %+v, want the operator as actor", allowed)
	}
	if refusal := events[1]; refusal.level != logclient.Warning || refusal.payload["error"] != "authentication required" {
		t.Fatalf("refusal event = %+v", refusal)
	}
	if outage := events[2]; outage.level != logclient.Error || outage.payload["error"] != "auth /v1/session returned HTTP 500" {
		t.Fatalf("outage event = %+v", outage)
	}
}

// /v1/auth/status tells the UI which of Exo's permissions the session holds.
func TestAuthStatus(t *testing.T) {
	handler, stub := gatedHandler(t, &recorder{})
	do(t, handler, http.MethodGet, "/v1/auth/status", nil).expect(t, http.StatusOK,
		`{"can_live":false,"can_historical":false,"can_commands":false,"permissions":{"commands":false,"historical":false,"live":false}}`, nil)

	stub.SignIn(authclient.Operator{Login: "guest", IsGuest: true, GuestAppID: auth.AppID}, auth.PermissionLive, auth.PermissionHistorical)
	got := do(t, handler, http.MethodGet, "/v1/auth/status", map[string]string{"Cookie": "oauth_session=guest"})
	if got.status != http.StatusOK || !strings.Contains(got.body, `"login":"guest"`) ||
		!strings.Contains(got.body, `"can_live":true,"can_historical":true,"can_commands":false`) {
		t.Fatalf("guest status = %d %s", got.status, got.body)
	}
}

// resolvedIP serves one request through a real listener — the trusted-proxy
// check is entirely about who the peer is — and reports the client IP the
// server resolved, which the test route sends back in a header.
func resolvedIP(t *testing.T, trustedProxies []string, proxyHeader string) string {
	t.Helper()
	handler := newHandler(frontend, trustedProxies, &recorder{}, authclient.New("http://oauth-manager:8080", auth.AppID), func(mux *http.ServeMux, _ gated) {
		mux.HandleFunc("GET /who", func(w http.ResponseWriter, r *http.Request) { w.Header().Set("X-Client-Ip", ClientIP(r)) })
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/who", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cf-Connecting-Ip", proxyHeader)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = response.Body.Close()
	return response.Header.Get("X-Client-Ip")
}

// Per-IP rate limiting is only per-IP if the server resolves the real client
// address out of Cf-Connecting-Ip. If this regresses, every limiter silently
// collapses into a single bucket keyed on the edge proxy.
func TestTrustedProxyResolvesTheRealClientIP(t *testing.T) {
	if got := resolvedIP(t, []string{"127.0.0.1", "::1", "172.16.0.0/12"}, "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want 203.0.113.7", got)
	}
	// A CIDR covers the peer as well as a bare address does.
	if got := resolvedIP(t, []string{"127.0.0.0/8"}, "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("ClientIP via CIDR = %q, want 203.0.113.7", got)
	}
}

// The other half of the same guarantee: a peer that is not a configured proxy
// must not be able to name its own IP.
func TestUntrustedPeerCannotSpoofTheProxyHeader(t *testing.T) {
	if got := resolvedIP(t, []string{"198.51.100.9"}, "203.0.113.7"); got != "127.0.0.1" {
		t.Fatalf("ClientIP = %q, want the real peer 127.0.0.1", got)
	}
}

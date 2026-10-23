package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

const frontend = "http://localhost:5174"

type event struct {
	level   logclient.Level
	payload map[string]any
}

// recorder stands in for Logger.
type recorder struct {
	mu     sync.Mutex
	events []event
}

func (r *recorder) LogAsync(level logclient.Level, _ string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event{level, payload.(map[string]any)})
}

func (r *recorder) take() []event {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := r.events
	r.events = nil
	return events
}

// testHandler is New with extra routes mounted inside the same middleware.
func testHandler(sink *recorder, authz *auth.Client, extra func(*http.ServeMux)) http.Handler {
	if authz == nil {
		authz = auth.NewClient("http://oauth-manager:8080")
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
	head, err := http.Head(srv.URL + "/health")
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
// did, and Logger hears about it as an error.
func TestPanicIsAnswered500AndLogged(t *testing.T) {
	sink := &recorder{}
	handler := testHandler(sink, nil, func(mux *http.ServeMux) {
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
	handler := testHandler(&recorder{}, nil, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			read = len(body)
			_, _ = w.Write(body)
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(strings.Repeat("a", bodyLimit+1)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || rec.Body.String() != "Request Entity Too Large" {
		t.Fatalf("declared oversize body: %d %q", rec.Code, rec.Body.String())
	}
	if read != 0 {
		t.Fatal("the handler ran for a body over the limit")
	}

	// A body that hides its length is cut off at the limit instead.
	req = httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(strings.Repeat("a", bodyLimit+1)))
	req.ContentLength = -1
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if read != bodyLimit {
		t.Fatalf("handler read %d bytes of an undeclared body, want the %d-byte limit", read, bodyLimit)
	}
}

// The gate's answers pass through the stack unchanged, and an Auth outage
// reaches Logger as an error carrying its cause.
func TestGatedRouteThroughTheStack(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "s=ok" {
			_, _ = io.WriteString(w, `{"allowed":true}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	authz := auth.NewClient(upstream.URL)
	sink := &recorder{}
	handler := testHandler(sink, authz, func(mux *http.ServeMux) {
		mux.Handle("GET /gated", authz.Require(auth.PermissionLive)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, r, http.StatusOK, map[string]any{"ok": true})
		})))
	})
	silenceLog(t)

	do(t, handler, http.MethodGet, "/gated", map[string]string{"Cookie": "s=ok", "Origin": frontend}).
		expect(t, http.StatusOK, `{"ok":true}`, map[string]string{"Access-Control-Allow-Origin": frontend})
	do(t, handler, http.MethodGet, "/gated", nil).
		expect(t, http.StatusUnauthorized, `{"error":"authentication required"}`, nil)
	do(t, handler, http.MethodGet, "/gated", map[string]string{"Cookie": "s=broken"}).
		expect(t, http.StatusServiceUnavailable, `{"error":"authorization service unavailable"}`, nil)

	events := sink.take()
	if len(events) != 3 {
		t.Fatalf("events = %+v", events)
	}
	if refusal := events[1]; refusal.level != logclient.Warning || refusal.payload["error"] != "authentication required" {
		t.Fatalf("refusal event = %+v", refusal)
	}
	if outage := events[2]; outage.level != logclient.Error || outage.payload["error"] != "OAuthManager /v1/check returned HTTP 500" {
		t.Fatalf("outage event = %+v", outage)
	}
}

// resolvedIP serves one request through a real listener — the trusted-proxy
// check is entirely about who the peer is — and reports the client IP the
// server resolved.
func resolvedIP(t *testing.T, trustedProxies []string, proxyHeader string) string {
	t.Helper()
	handler := newHandler(frontend, trustedProxies, &recorder{}, auth.NewClient("http://oauth-manager:8080"), func(mux *http.ServeMux) {
		mux.HandleFunc("GET /who", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, ClientIP(r)) })
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	request, err := http.NewRequest(http.MethodGet, srv.URL+"/who", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cf-Connecting-Ip", proxyHeader)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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

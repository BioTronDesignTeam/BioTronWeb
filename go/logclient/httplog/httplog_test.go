package httplog

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

type event struct {
	level   logclient.Level
	message string
	payload map[string]any
}

type recorder struct {
	mu     sync.Mutex
	events []event
}

func (r *recorder) LogAsync(level logclient.Level, message string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fields, _ := payload.(map[string]any)
	r.events = append(r.events, event{level, message, fields})
}

func newHandler(sink Sink, options Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		SetActor(r, "octocat")
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /quiet", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("GET /quiet-fails", func(w http.ResponseWriter, r *http.Request) {
		SetError(r, errors.New("not signed in"))
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
	mux.HandleFunc("GET /missing", func(w http.ResponseWriter, r *http.Request) {
		SetError(r, errors.New("thing not found"))
		http.Error(w, "mapped", http.StatusNotFound)
	})
	mux.HandleFunc("GET /broken", func(w http.ResponseWriter, r *http.Request) {
		SetError(r, errors.New("database on fire"))
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /self-written", func(w http.ResponseWriter, r *http.Request) {
		SetError(r, errors.New("upstream said no"))
		w.WriteHeader(http.StatusBadGateway)
	})
	mux.HandleFunc("OPTIONS /ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /panics", func(w http.ResponseWriter, r *http.Request) { panic("boom") })
	return New(sink, options)(mux)
}

func get(t *testing.T, handler http.Handler, method, path string) (int, string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	body, _ := io.ReadAll(response.Result().Body)
	return response.Code, string(body)
}

func TestLogsStatusActorAndCause(t *testing.T) {
	sink := &recorder{}
	handler := newHandler(sink, Options{Quiet: []string{"/quiet", "/quiet-fails"}})

	get(t, handler, "GET", "/health")
	get(t, handler, "GET", "/quiet")
	get(t, handler, "OPTIONS", "/ok")
	if len(sink.events) != 0 {
		t.Fatalf("health, quiet, and preflight must not log; got %+v", sink.events)
	}

	if status, _ := get(t, handler, "GET", "/ok"); status != 200 {
		t.Fatalf("status = %d", status)
	}
	if status, _ := get(t, handler, "GET", "/quiet-fails"); status != 401 {
		t.Fatalf("status = %d", status)
	}
	if status, body := get(t, handler, "GET", "/missing"); status != 404 || body != "mapped\n" {
		t.Fatalf("status = %d body = %q", status, body)
	}
	if status, _ := get(t, handler, "GET", "/broken"); status != 500 {
		t.Fatalf("status = %d", status)
	}
	get(t, handler, "GET", "/self-written")

	want := []struct {
		level  logclient.Level
		path   string
		status int
		actor  string
		cause  string
	}{
		{logclient.Info, "/ok", 200, "octocat", ""},
		{logclient.Warning, "/quiet-fails", 401, "", "not signed in"},
		{logclient.Warning, "/missing", 404, "", "thing not found"},
		{logclient.Error, "/broken", 500, "", "database on fire"},
		{logclient.Error, "/self-written", 502, "", "upstream said no"},
	}
	if len(sink.events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(sink.events), len(want), sink.events)
	}
	for i, w := range want {
		got := sink.events[i]
		if got.message != "HTTP request completed" || got.level != w.level || got.payload["path"] != w.path || got.payload["status"] != w.status {
			t.Errorf("event %d = %+v, want %+v", i, got, w)
		}
		if actor, _ := got.payload["actor"].(string); actor != w.actor {
			t.Errorf("event %d actor = %q, want %q", i, actor, w.actor)
		}
		if cause, _ := got.payload["error"].(string); cause != w.cause {
			t.Errorf("event %d error = %q, want %q", i, cause, w.cause)
		}
		if _, ok := got.payload["duration_ms"].(int64); !ok {
			t.Errorf("event %d has no duration", i)
		}
	}
}

func TestSkipDropsRefusedRequests(t *testing.T) {
	sink := &recorder{}
	handler := newHandler(sink, Options{Skip: func(r *http.Request, status int) bool {
		return r.URL.Path == "/quiet-fails" && status == http.StatusUnauthorized
	}})
	get(t, handler, "GET", "/quiet-fails")
	get(t, handler, "GET", "/ok")
	if len(sink.events) != 1 || sink.events[0].payload["path"] != "/ok" {
		t.Fatalf("skip must drop the refused request only; got %+v", sink.events)
	}
}

func TestPanicIsAnsweredAndLoggedAsA500(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	sink := &recorder{}
	handler := newHandler(sink, Options{})

	if status, _ := get(t, handler, "GET", "/panics"); status != 500 {
		t.Fatalf("status = %d", status)
	}
	if len(sink.events) != 1 {
		t.Fatalf("got %d events: %+v", len(sink.events), sink.events)
	}
	if got := sink.events[0]; got.level != logclient.Error || got.payload["status"] != 500 || got.payload["error"] != "boom" {
		t.Errorf("panic must be logged as a 500 with its cause; got %+v", got)
	}
}

func TestAbortHandlerIsLoggedAndReRaised(t *testing.T) {
	sink := &recorder{}
	handler := New(sink, Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if recovered := recover(); !IsAbort(recovered) {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", recovered)
		}
		if len(sink.events) != 1 || sink.events[0].payload["status"] != 500 {
			t.Fatalf("abort must be logged as a 500; got %+v", sink.events)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/aborts", nil))
}

func TestHelpersAreNoopsOutsideTheMiddleware(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	SetError(request, errors.New("ignored"))
	SetActor(request, "ignored")
	SetError(nil, errors.New("ignored"))
	SetActor(nil, "ignored")
}

func TestFlushIsPassedThrough(t *testing.T) {
	sink := &recorder{}
	handler := New(sink, Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("the wrapped writer must still be an http.Flusher")
		}
		flusher.Flush()
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if !response.Flushed {
		t.Fatal("flush did not reach the underlying writer")
	}
	if len(sink.events) != 1 || sink.events[0].payload["status"] != 200 {
		t.Fatalf("got %+v", sink.events)
	}
}

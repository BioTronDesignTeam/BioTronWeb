// Package httplog sends one event to Logger for each request a net/http
// server serves: method, path, status, and duration, plus the cause when the
// request failed and the operator when the app names one. Every backend
// mounts it, so request events read the same in the log explorer whichever
// service sent them.
package httplog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// Sink receives the events. *logclient.Client satisfies it; Logger itself
// passes an adapter that writes straight into its store.
type Sink interface {
	LogAsync(level logclient.Level, message string, payload any)
}

type Options struct {
	// Quiet paths are logged only when they fail. Use it for routes a page
	// polls on every load, such as a session check.
	Quiet []string
	// Skip drops a completed request from the log when it returns true. It
	// runs after the status is known, so a service can keep an attacker's
	// refused requests from writing into the audit trail.
	Skip func(r *http.Request, status int) bool
}

type stateKey struct{}

// state is what handlers attach to a request for its completion event. A
// handler may hand the request to another goroutine, so it is locked.
type state struct {
	mu    sync.Mutex
	err   string
	actor string
}

func fromRequest(r *http.Request) *state {
	if r == nil {
		return nil
	}
	s, _ := r.Context().Value(stateKey{}).(*state)
	return s
}

// SetError attaches the cause of a failure to the request so the completion
// event carries it. net/http handlers return no error, so call it wherever a
// handler writes an error status. It does nothing outside the middleware.
func SetError(r *http.Request, err error) {
	if s := fromRequest(r); s != nil && err != nil {
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
	}
}

// SetActor names the signed-in operator on the completion event. It does
// nothing outside the middleware.
func SetActor(r *http.Request, login string) {
	if s := fromRequest(r); s != nil && login != "" {
		s.mu.Lock()
		s.actor = login
		s.mu.Unlock()
	}
}

// IsAbort reports whether a recovered panic value is http.ErrAbortHandler,
// the panic a handler raises to abort a response on purpose.
func IsAbort(recovered any) bool {
	err, ok := recovered.(error)
	return ok && errors.Is(err, http.ErrAbortHandler)
}

// New returns the middleware. Mount it outermost, so its status is the one
// the client received. It also recovers panics: a panic is answered with a
// 500 when nothing was written yet, printed with its stack on the standard
// log, and logged as the 500 it became. http.ErrAbortHandler is re-raised
// after logging, so net/http still aborts the response as the handler asked.
func New(sink Sink, options Options) func(http.Handler) http.Handler {
	quiet := make(map[string]bool, len(options.Quiet))
	for _, path := range options.Quiet {
		quiet[path] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			s := &state{}
			r = r.WithContext(context.WithValue(r.Context(), stateKey{}, s))
			recorder := &statusRecorder{ResponseWriter: w}

			defer func() {
				recovered := recover()
				if recovered != nil {
					if !IsAbort(recovered) {
						log.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, recovered, debug.Stack())
					}
					s.mu.Lock()
					s.err = fmt.Sprint(recovered)
					s.mu.Unlock()
					if !recorder.wroteHeader {
						recorder.WriteHeader(http.StatusInternalServerError)
					} else {
						// The client already has a status; what it gets is
						// a cut-off body, which is a server failure.
						recorder.status = http.StatusInternalServerError
					}
				}
				complete(sink, options, quiet, r, recorder.statusCode(), s, started)
				if IsAbort(recovered) {
					panic(recovered)
				}
			}()
			next.ServeHTTP(recorder, r)
		})
	}
}

func complete(sink Sink, options Options, quiet map[string]bool, r *http.Request, status int, s *state, started time.Time) {
	path := r.URL.Path
	if path == "/health" {
		return
	}
	if status < 400 && (r.Method == http.MethodOptions || quiet[path]) {
		return
	}
	if options.Skip != nil && options.Skip(r, status) {
		return
	}

	level := logclient.Info
	if status >= 500 {
		level = logclient.Error
	} else if status >= 400 {
		level = logclient.Warning
	}
	payload := map[string]any{
		"method":      r.Method,
		"path":        path,
		"status":      status,
		"duration_ms": time.Since(started).Milliseconds(),
	}
	s.mu.Lock()
	if s.actor != "" {
		payload["actor"] = s.actor
	}
	if s.err != "" {
		payload["error"] = s.err
	}
	s.mu.Unlock()
	sink.LogAsync(level, "HTTP request completed", payload)
}

// statusRecorder remembers the status the handler sent. It passes Flush and
// Hijack through so streaming and WebSocket handlers keep working, and
// Unwrap lets http.ResponseController reach the rest.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(code int) {
	// 1xx responses other than 101 are interim; the final status follows.
	if !w.wroteHeader && (code >= 200 || code == http.StatusSwitchingProtocols) {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusRecorder) Flush() {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buffer, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && !w.wroteHeader {
		w.status = http.StatusSwitchingProtocols
		w.wroteHeader = true
	}
	return conn, buffer, err
}

func (w *statusRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// statusCode is what the client received: 200 when the handler wrote
// nothing, as net/http itself sends.
func (w *statusRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

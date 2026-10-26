package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// bodyLimit is the largest request body the API reads.
const bodyLimit = 256 * 1024

// handlerFunc is a route that may fail. serve answers a returned error with
// the JSON error body and attaches it to the request's log event.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h *Handler) serve(handle handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := handle(w, r); err != nil {
			httplog.SetError(r, err)
			writeError(w, err)
		}
	})
}

// httpError is a refusal with a status of its own. The message is the
// standard status text; the client sees it in lower case.
type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func newHTTPError(status int) *httpError {
	return &httpError{status: status, message: http.StatusText(status)}
}

var (
	errBadRequest         = newHTTPError(http.StatusBadRequest)
	errUnauthorized       = newHTTPError(http.StatusUnauthorized)
	errForbidden          = newHTTPError(http.StatusForbidden)
	errNotFound           = newHTTPError(http.StatusNotFound)
	errBodyTooLarge       = newHTTPError(http.StatusRequestEntityTooLarge)
	errServiceUnavailable = newHTTPError(http.StatusServiceUnavailable)
)

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	var refusal *httpError
	switch {
	case errors.As(err, &refusal):
		status = refusal.status
		message = strings.ToLower(refusal.message)
	case errors.Is(err, store.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		status, message = http.StatusConflict, "the requested change conflicts with existing calendar data"
	}
	if status >= 500 {
		log.Printf("request failed: %v", err)
	}
	_ = writeJSON(w, status, map[string]any{"error": message})
}

// writeJSON sends value as the whole body, with no trailing newline.
func writeJSON(w http.ResponseWriter, status int, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	writeBody(w, status, body)
	return nil
}

// writeBody sets Content-Length itself, so a HEAD answer carries the length
// of the body it leaves out, whatever its size.
func writeBody(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// decodeBody reads a JSON request body into out. A body that is not declared
// as JSON, or does not parse, is a 400; one over bodyLimit is a 413.
func decodeBody(r *http.Request, out any) error {
	if !isJSON(r.Header.Get("Content-Type")) {
		return errBadRequest
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil {
		return errBadRequest
	}
	if len(body) > bodyLimit {
		return errBodyTooLarge
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errBadRequest
	}
	return nil
}

// isJSON reads a Content-Type the way Fiber's binder did: case-insensitive,
// parameters ignored, and a vendor type such as application/vnd.api+json
// counted as JSON.
func isJSON(contentType string) bool {
	mediaType := strings.ToLower(contentType)
	if i := strings.IndexAny(mediaType, " ;"); i >= 0 {
		mediaType = mediaType[:i]
	}
	if slash := strings.IndexByte(mediaType, '/'); slash >= 0 {
		if plus := strings.LastIndexByte(mediaType, '+'); plus > slash {
			mediaType = mediaType[:slash+1] + mediaType[plus+1:]
		}
	}
	return mediaType == "application/json"
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, errNotFound)
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	_ = writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
}

// routeJSON serves a mux, but answers a path it does not know, or a method
// it does not route, with the JSON error body instead of the mux's text.
func routeJSON(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		probe := &probeWriter{header: http.Header{}}
		handler.ServeHTTP(probe, r)
		if probe.status == http.StatusMethodNotAllowed {
			methodNotAllowed(w, fiberMethodOrder(probe.header.Get("Allow")))
			return
		}
		notFound(w, r)
	})
}

// probeWriter records what the mux's own 404 or 405 handler would send.
type probeWriter struct {
	header http.Header
	status int
}

func (p *probeWriter) Header() http.Header         { return p.header }
func (p *probeWriter) Write(b []byte) (int, error) { return len(b), nil }
func (p *probeWriter) WriteHeader(status int)      { p.status = status }

// fiberMethodOrder lists the methods of an Allow header in the order Fiber
// listed them.
func fiberMethodOrder(allow string) string {
	listed := map[string]bool{}
	for _, method := range strings.Split(allow, ",") {
		listed[strings.TrimSpace(method)] = true
	}
	var ordered []string
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch,
	} {
		if listed[method] {
			ordered = append(ordered, method)
		}
	}
	return strings.Join(ordered, ", ")
}

// fiberPaths routes a path as Fiber did: without regard to case and to
// trailing slashes. A path that is not clean, such as //v1/scopes, is a 404
// rather than a redirect. The request log keeps the path as it was sent.
func fiberPaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routed := strings.TrimRight(strings.ToLower(r.URL.Path), "/")
		if routed == "" {
			routed = "/"
		}
		if path.Clean(routed) != routed {
			notFound(w, r)
			return
		}
		if routed != r.URL.Path {
			url := *r.URL
			url.Path, url.RawPath = routed, ""
			r = r.WithContext(r.Context())
			r.URL = &url
		}
		next.ServeHTTP(w, r)
	})
}

// recoverJSON answers a panic with the JSON 500 every other failure gets, and
// prints the stack.
func recoverJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if httplog.IsAbort(recovered) {
				panic(recovered)
			}
			// The path is decoded, so it can hold a line break. Escaped, it
			// cannot forge a line of its own.
			request := strings.ReplaceAll(strings.ReplaceAll(r.Method+" "+r.URL.Path, "\r", `\r`), "\n", `\n`)
			log.Printf("panic serving %s: %v\n%s", request, recovered, debug.Stack())
			err := fmt.Errorf("panic: %v", recovered)
			httplog.SetError(r, err)
			writeError(w, err)
		}()
		next.ServeHTTP(w, r)
	})
}

func isAdminPath(routed string) bool {
	return routed == "/v1/admin" || strings.HasPrefix(routed, "/v1/admin/")
}

// feedFile passes on only a file name that ends in .ics. Anything else under
// /v1/feeds/scopes/ matched no GET route under Fiber, only the OPTIONS one.
func feedFile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		if len(file) <= len(".ics") || !strings.HasSuffix(file, ".ics") {
			methodNotAllowed(w, http.MethodOptions)
			return
		}
		next.ServeHTTP(w, r)
	})
}

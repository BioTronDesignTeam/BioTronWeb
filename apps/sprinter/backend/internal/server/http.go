package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// handlerFunc is a handler that may fail. Returning the error, rather than
// writing it, keeps one place that turns errors into statuses and one place
// that tells the request log why a request failed.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func handle(fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			writeError(w, r, err)
		}
	})
}

// httpError is an error with the status it answers. Its message is what the
// log records; the body carries it in lower case.
type httpError struct {
	status  int
	message string
	// cause, when set, is what the log records instead: a 503 should say
	// what was unavailable, not only that something was.
	cause error
}

func (e *httpError) Error() string { return e.message }

func (e *httpError) because(cause error) *httpError {
	return &httpError{status: e.status, message: e.message, cause: cause}
}

var (
	errUnauthorized       = &httpError{status: http.StatusUnauthorized, message: "Unauthorized"}
	errForbidden          = &httpError{status: http.StatusForbidden, message: "Forbidden"}
	errNotFound           = &httpError{status: http.StatusNotFound, message: "Not Found"}
	errMethodNotAllowed   = &httpError{status: http.StatusMethodNotAllowed, message: "Method Not Allowed"}
	errBodyTooLarge       = &httpError{status: http.StatusRequestEntityTooLarge, message: "Request Entity Too Large"}
	errServiceUnavailable = &httpError{status: http.StatusServiceUnavailable, message: "Service Unavailable"}
)

func badRequest(message string) error {
	return &httpError{status: http.StatusBadRequest, message: message}
}

// writeError answers {"error": "..."} with the status the error maps to. A
// store miss is a 404 and a constraint is a 409; anything else is a 500 that
// names nothing internal.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	logged := err
	var known *httpError
	switch {
	case errors.As(err, &known):
		status = known.status
		message = strings.ToLower(known.message)
		if known.cause != nil {
			logged = known.cause
		}
	case errors.Is(err, store.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		status, message = http.StatusConflict, "the requested change conflicts with existing data"
	}
	if status >= 500 {
		log.Printf("request failed: %v", err)
	}
	httplog.SetError(r, logged)
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		log.Printf("encode response: %v", err)
		status, raw = http.StatusInternalServerError, []byte(`{"error":"internal server error"}`)
	}
	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Content-Length", fmt.Sprint(len(raw)))
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

// bind decodes a JSON body. A malformed body is the caller's fault, so it is
// a 400 rather than the 500 a decoder error would read as.
func bind(r *http.Request, destination any) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errBodyTooLarge
		}
		return badRequest("body must be a JSON object")
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return badRequest("body must be a JSON object")
	}
	return nil
}

func notFound(http.ResponseWriter, *http.Request) error {
	return errNotFound
}

// methods answers a known path asked with a method it does not serve. The
// Allow list names what the path does serve, HEAD included with GET.
func methods(allow string) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Allow", allow)
		return errMethodNotAllowed
	})
}

// adminMethods is methods below /v1/admin, where every path has a preflight
// route: an OPTIONS that is not a preflight matched that route and found
// nothing behind it, which is a 404, not a wrong method.
func adminMethods(allow string) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method == http.MethodOptions {
			return errNotFound
		}
		w.Header().Set("Allow", allow)
		return errMethodNotAllowed
	})
}

// newCORS lets the admin page's origins make credentialed requests. A
// preflight is answered here with 204 and goes no further. Every other
// request gets Vary: Origin, and the allow headers when its origin is listed.
func newCORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[strings.ToLower(strings.TrimSpace(origin))] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			origin := r.Header.Get("Origin")
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if origin == "" || !preflight {
				header.Add("Vary", "Origin")
				// An OPTIONS that is not a preflight is passed on bare.
				if origin != "" && r.Method != http.MethodOptions && allowed[strings.ToLower(origin)] {
					header.Set("Access-Control-Allow-Origin", origin)
					header.Set("Access-Control-Allow-Credentials", "true")
				}
				next.ServeHTTP(w, r)
				return
			}
			header.Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Origin")
			if allowed[strings.ToLower(origin)] {
				header.Set("Access-Control-Allow-Origin", origin)
				header.Set("Access-Control-Allow-Credentials", "true")
			}
			header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			header.Set("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// normalizePath routes /health/ and /HEALTH as /health, as the routes have
// always matched: no trailing slash, any case. The request keeps its own
// path everywhere else, so the log shows what the client asked for.
func normalizePath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.ToLower(r.URL.Path)
		if len(path) > 1 {
			path = strings.TrimRight(path, "/")
			if path == "" {
				path = "/"
			}
		}
		if path != r.URL.Path {
			routed := r.Clone(r.Context())
			routed.URL.Path, routed.URL.RawPath = path, ""
			r = routed
		}
		next.ServeHTTP(w, r)
	})
}

// limitBody refuses a body over limit before any handler reads it, and caps
// one that did not say its length.
func limitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				w.Header().Set("Connection", "close")
				writeError(w, r, errBodyTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// recoverJSON answers a panic with the same JSON 500 as any other failure,
// then panics again so httplog, outside it, logs the stack and the 500.
func recoverJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered != http.ErrAbortHandler && recorder.status == 0 {
					writeJSON(recorder, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
				}
				panic(recovered)
			}
		}()
		next.ServeHTTP(recorder, r)
	})
}

// accessLog prints one console line per request, for `docker logs`. The
// client address is the Cf-Connecting-Ip header only when the peer is one of
// trustedProxies; from anyone else the header is a claim, not an address.
func accessLog(trustedProxies []string) func(http.Handler) http.Handler {
	trusted := parseProxies(trustedProxies)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}
			defer func() {
				status := recorder.status
				if status == 0 {
					status = http.StatusOK
				}
				log.Printf("%3d | %13v | %15s | %-7s | %s",
					status, time.Since(started), clientIP(r, trusted), r.Method, r.URL.Path)
			}()
			next.ServeHTTP(recorder, r)
		})
	}
}

func parseProxies(values []string) []netip.Prefix {
	var prefixes []netip.Prefix
	for _, value := range values {
		if prefix, err := netip.ParsePrefix(value); err == nil {
			prefixes = append(prefixes, prefix.Masked())
		} else if addr, err := netip.ParseAddr(value); err == nil {
			prefixes = append(prefixes, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
		}
	}
	return prefixes
}

func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	for _, prefix := range trusted {
		if prefix.Contains(peer.Unmap()) {
			if forwarded := strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip")); forwarded != "" {
				return forwarded
			}
			break
		}
	}
	return host
}

// statusRecorder remembers the status written through it. Unwrap lets
// http.ResponseController reach the writer underneath.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	if w.status == 0 && code >= 200 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

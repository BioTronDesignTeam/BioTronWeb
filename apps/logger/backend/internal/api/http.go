package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// The plumbing net/http leaves to the app: handlers that return an error, one
// JSON error shape, the client address behind the edge, and a per-address
// limiter. Each piece answers exactly as the Fiber app it replaced did, so the
// status page, the log explorer, and every logclient sender see no change.

// httpError is an error with the status and message the client receives.
// Anything else a handler returns is a 500 whose cause stays in the log.
type httpError struct {
	code    int
	message string
}

func (e *httpError) Error() string { return e.message }

func newError(code int, message ...string) *httpError {
	err := &httpError{code: code, message: http.StatusText(code)}
	if len(message) > 0 {
		err.message = message[0]
	}
	return err
}

var (
	errUnauthorized = newError(http.StatusUnauthorized)
	errForbidden    = newError(http.StatusForbidden)
	errNotFound     = newError(http.StatusNotFound)
)

// handlerFunc is a handler that may fail. The adapter turns the failure into
// the JSON error and hands its cause to the request log.
type handlerFunc func(http.ResponseWriter, *http.Request) error

func handle(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, r, err)
		}
	})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := http.StatusInternalServerError
	message := "internal server error"
	var known *httpError
	if errors.As(err, &known) {
		code = known.code
		message = known.message
	} else {
		// The path is decoded, so it can hold a line break. Escaped, it
		// cannot forge a line of its own.
		request := strings.ReplaceAll(strings.ReplaceAll(r.Method+" "+r.URL.Path, "\r", `\r`), "\n", `\n`)
		log.Printf("request %s: %v", request, err)
	}
	httplog.SetError(r, err)
	writeJSON(w, code, map[string]any{"error": message})
}

// writeJSON sends value with encoding/json's Marshal, as Fiber did: no
// trailing newline, and HTML characters escaped.
func writeJSON(w http.ResponseWriter, code int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		log.Printf("encode response: %v", err)
		code = http.StatusInternalServerError
		raw = []byte(`{"error":"internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}

func writeText(w http.ResponseWriter, code int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(text))
}

// recoverPanics answers a panic with the same JSON 500 as any other failure,
// and names the panic as the request's error. It sits inside httplog, so the
// request log records the 500 the caller got.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if httplog.IsAbort(recovered) {
				panic(recovered)
			}
			err, ok := recovered.(error)
			if !ok {
				err = fmt.Errorf("%v", recovered)
			}
			writeError(w, r, err)
		}()
		next.ServeHTTP(w, r)
	})
}

// lenientPaths routes "/v1/status/" as "/v1/status", as Fiber's default
// non-strict routing did. Only routing sees the trimmed path; the request log
// still records the path the client sent.
func lenientPaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			trimmed := strings.TrimRight(path, "/")
			if trimmed == "" {
				trimmed = "/"
			}
			routed := *r
			address := *r.URL
			address.Path = trimmed
			address.RawPath = ""
			routed.URL = &address
			r = &routed
		}
		next.ServeHTTP(w, r)
	})
}

// methodsInOrder is the order Fiber listed methods in an Allow header.
var methodsInOrder = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
	http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch,
}

// unmatched answers a request no route took: 405 with an Allow header when
// the path exists under another method, 404 otherwise. ServeMux would answer
// both in plain text; the API answers every error in JSON.
func unmatched(mux *http.ServeMux, fallbacks ...string) handlerFunc {
	isFallback := make(map[string]bool, len(fallbacks))
	for _, pattern := range fallbacks {
		isFallback[pattern] = true
	}
	return func(w http.ResponseWriter, r *http.Request) error {
		allowed := make([]string, 0, 2)
		for _, method := range methodsInOrder {
			probe := *r
			probe.Method = method
			if _, pattern := mux.Handler(&probe); pattern != "" && !isFallback[pattern] {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) == 0 {
			return errNotFound
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		return newError(http.StatusMethodNotAllowed)
	}
}

// proxyTrust decides which peers may name the client in Cf-Connecting-Ip.
// Entries are single addresses or CIDR ranges, as TRUSTED_PROXIES spells them.
type proxyTrust struct {
	addresses map[netip.Addr]bool
	ranges    []netip.Prefix
}

func newProxyTrust(entries []string) proxyTrust {
	trust := proxyTrust{addresses: make(map[netip.Addr]bool)}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			prefix, err := netip.ParsePrefix(entry)
			if err != nil {
				log.Printf("ignoring trusted proxy %q: %v", entry, err)
				continue
			}
			trust.ranges = append(trust.ranges, prefix.Masked())
			continue
		}
		address, err := netip.ParseAddr(entry)
		if err != nil {
			log.Printf("ignoring trusted proxy %q: %v", entry, err)
			continue
		}
		trust.addresses[address.Unmap()] = true
	}
	return trust
}

func (t proxyTrust) trusts(address netip.Addr) bool {
	if t.addresses[address] {
		return true
	}
	for _, prefix := range t.ranges {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// clientIP is the address every limiter keys on. A trusted peer's
// Cf-Connecting-Ip is taken as it stands, even when absent, exactly as Fiber
// did without IP validation; anyone else is judged by the peer address.
func (t proxyTrust) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if t.trusts(peer) {
		return strings.Join(r.Header.Values("Cf-Connecting-Ip"), ", ")
	}
	return peer.String()
}

// limiter allows max requests per key in fixed windows, counted in whole
// seconds. It sends the X-RateLimit-* headers on every request it lets
// through and Retry-After on the 429, the same headers Fiber's limiter sent.
type limiter struct {
	max     int
	window  int64
	key     func(*http.Request) string
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]*limitEntry
	sweptAt int64
}

type limitEntry struct {
	hits      int
	expiresAt int64
}

func newLimiter(max int, window time.Duration, key func(*http.Request) string) *limiter {
	seconds := int64(window / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return &limiter{max: max, window: seconds, key: key, now: time.Now, entries: make(map[string]*limitEntry)}
}

func (l *limiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := l.key(r)
		now := l.now().Unix()

		l.mu.Lock()
		// Forget finished windows once a window, so a stream of one-off
		// addresses cannot grow the map without bound.
		if now-l.sweptAt >= l.window {
			for k, entry := range l.entries {
				if now >= entry.expiresAt {
					delete(l.entries, k)
				}
			}
			l.sweptAt = now
		}
		entry, ok := l.entries[key]
		if !ok || now >= entry.expiresAt {
			entry = &limitEntry{expiresAt: now + l.window}
			l.entries[key] = entry
		}
		entry.hits++
		resetIn := entry.expiresAt - now
		remaining := l.max - entry.hits
		l.mu.Unlock()

		if remaining < 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(resetIn, 10))
			writeText(w, http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
			return
		}
		header := w.Header()
		header.Set("X-RateLimit-Limit", strconv.Itoa(l.max))
		header.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		header.Set("X-RateLimit-Reset", strconv.FormatInt(resetIn, 10))
		next.ServeHTTP(w, r)
	})
}

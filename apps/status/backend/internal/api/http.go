package api

import (
	"encoding/json"
	"errors"
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
// limiter.

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

// writeJSON sends value as the whole body.
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

// clientIP is the address every limiter keys on: a trusted peer's
// Cf-Connecting-Ip when it holds an address, the peer otherwise.
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
		if client, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip"))); err == nil {
			return client.Unmap().String()
		}
	}
	return peer.String()
}

// limiter allows max requests per key in fixed windows, counted in whole
// seconds. A refused request gets 429 with Retry-After.
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
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

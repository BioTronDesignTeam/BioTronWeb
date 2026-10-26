package server

import (
	"log"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/httpx"
)

// limiter allows each client max requests per window. The window is fixed: it
// starts at a client's first request and the count resets when it ends. Past
// the limit it answers 429 with Retry-After. One process serves Auth, so memory is enough.
type limiter struct {
	max    int
	window time.Duration
	key    func(*http.Request) string
	now    func() time.Time

	mu      sync.Mutex
	clients map[string]*window
	swept   time.Time
}

type window struct {
	hits   int
	resets time.Time
}

func newLimiter(limit int, per time.Duration, key func(*http.Request) string) *limiter {
	return &limiter{max: limit, window: per, key: key, now: time.Now, clients: map[string]*window{}}
}

// hit counts one request of key and returns its count in the current window
// and when that window ends.
func (l *limiter) hit(key string) (int, time.Time) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	// Forget finished windows once per window, so the map holds only the
	// clients seen lately.
	if now.Sub(l.swept) >= l.window {
		for k, w := range l.clients {
			if !now.Before(w.resets) {
				delete(l.clients, k)
			}
		}
		l.swept = now
	}
	w := l.clients[key]
	if w == nil || !now.Before(w.resets) {
		w = &window{resets: now.Add(l.window)}
		l.clients[key] = w
	}
	w.hits++
	return w.hits, w.resets
}

func (l *limiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits, resets := l.hit(l.key(r))
		if hits > l.max {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(resets.Sub(l.now()).Seconds()))))
			httpx.Status(w, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the key of a request's client: the Cf-Connecting-Ip header
// when the request comes from one of the trusted proxies, else the peer
// address. The edge sets the header itself; from anyone else it is a forgery
// that could spend another client's budget or escape its own.
func clientIP(trustedProxies []string) func(*http.Request) string {
	var trusted []netip.Prefix
	for _, entry := range trustedProxies {
		entry = strings.TrimSpace(entry)
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			trusted = append(trusted, prefix.Masked())
		} else if addr, err := netip.ParseAddr(entry); err == nil {
			trusted = append(trusted, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
		} else {
			log.Printf("TRUSTED_PROXIES: ignoring %q, which is neither an address nor a CIDR range", entry)
		}
	}
	return func(r *http.Request) string {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		peer, err := netip.ParseAddr(host)
		if err != nil {
			return host
		}
		peer = peer.Unmap()
		for _, prefix := range trusted {
			if prefix.Contains(peer) {
				if forwarded := strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip")); forwarded != "" {
					return forwarded
				}
				break
			}
		}
		return peer.String()
	}
}

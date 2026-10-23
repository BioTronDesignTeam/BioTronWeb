package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// bodyLimit is the request body cap Fiber applied by default.
const bodyLimit = 4 << 20

// chain wraps h in middleware, the first one outermost.
func chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// writeJSON answers the way Fiber's c.JSON did: compact, no trailing newline,
// and the charset spelled out in the content type.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		httplog.SetError(r, err)
		writeText(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}
	write(w, status, "application/json; charset=utf-8", encoded)
}

// writeText answers the way Fiber's default error handler did: the message as
// plain text.
func writeText(w http.ResponseWriter, status int, message string) {
	write(w, status, "text/plain; charset=utf-8", []byte(message))
}

func write(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	// Spelled out so a HEAD answer carries it too, as Fiber's did.
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// fallback serves mux, but answers a request no route matches the way Fiber
// did: a plain-text 404 or 405 rather than net/http's own wording.
func fallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// The mux's own not-found handler knows whether another method
		// would have matched; ask it, keep its Allow header, drop its body.
		probe := &headerProbe{header: http.Header{}}
		handler.ServeHTTP(probe, r)
		switch probe.status {
		case http.StatusMethodNotAllowed:
			w.Header()["Allow"] = probe.header["Allow"]
			fail(w, r, http.StatusMethodNotAllowed)
		case http.StatusNotFound:
			fail(w, r, http.StatusNotFound)
		default:
			mux.ServeHTTP(w, r)
		}
	})
}

// fail answers status with its standard text and records that text as the
// cause, as Fiber's error handler did for its built-in errors.
func fail(w http.ResponseWriter, r *http.Request, status int) {
	message := http.StatusText(status)
	httplog.SetError(r, errors.New(message))
	writeText(w, status, message)
}

// headerProbe is a ResponseWriter that keeps only the status and headers.
type headerProbe struct {
	header http.Header
	status int
}

func (p *headerProbe) Header() http.Header { return p.header }

func (p *headerProbe) Write(b []byte) (int, error) {
	if p.status == 0 {
		p.status = http.StatusOK
	}
	return len(b), nil
}

func (p *headerProbe) WriteHeader(status int) {
	if p.status == 0 {
		p.status = status
	}
}

// limitBody refuses a body over limit with 413, as Fiber's BodyLimit did. A
// body that does not declare its length is cut off at limit instead.
func limitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				fail(w, r, http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// cors allows the one frontend origin with credentials, answering exactly as
// Fiber's cors middleware did with AllowOrigins: [origin], AllowCredentials,
// AllowMethods GET, POST, OPTIONS, and AllowHeaders Content-Type,
// X-Requested-With.
func cors(frontendURL string) func(http.Handler) http.Handler {
	allowed := normalizeOrigin(frontendURL)
	const (
		allowMethods = "GET, POST, OPTIONS"
		allowHeaders = "Content-Type, X-Requested-With"
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			origin := r.Header.Get("Origin")
			// No Origin is not a CORS request; one is still told the answer
			// varies by Origin so a cache does not reuse it for one that has.
			// Neither is an OPTIONS without Access-Control-Request-Method;
			// it falls through to the router.
			if origin == "" || (r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") == "") {
				header.Add("Vary", "Origin")
				next.ServeHTTP(w, r)
				return
			}

			allowOrigin := allowed != "" && strings.ToLower(origin) == allowed
			setAllowOrigin := func() {
				if allowOrigin {
					header.Set("Access-Control-Allow-Origin", origin)
					header.Set("Access-Control-Allow-Credentials", "true")
				}
			}

			if r.Method != http.MethodOptions {
				header.Add("Vary", "Origin")
				setAllowOrigin()
				next.ServeHTTP(w, r)
				return
			}

			// A preflight is answered here for any path, matched or not, and
			// the methods and headers are listed even to a refused origin; the
			// missing Allow-Origin is what refuses it.
			header.Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Origin")
			setAllowOrigin()
			header.Set("Access-Control-Allow-Methods", allowMethods)
			header.Set("Access-Control-Allow-Headers", allowHeaders)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// normalizeOrigin lowercases the scheme and host of a configured origin and
// drops any path, so it compares against a lowercased Origin header.
func normalizeOrigin(origin string) string {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		log.Printf("warning: FRONTEND_URL %q is not an origin; no cross-origin request will be allowed", origin)
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

type clientIPKey struct{}

// ClientIP is the address the request came from: Cf-Connecting-Ip when the
// peer is a trusted proxy, the peer itself otherwise.
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerIP(r)
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientIP resolves the real client address once per request. Without it
// every request appears to come from the edge proxy and any per-IP limit
// collapses into one bucket. Only the listed peers may name the client;
// anyone else could set Cf-Connecting-Ip to whatever they like.
func clientIP(trustedProxies []string) func(http.Handler) http.Handler {
	var trusted []netip.Prefix
	for _, entry := range trustedProxies {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			trusted = append(trusted, prefix.Masked())
		} else if addr, err := netip.ParseAddr(entry); err == nil {
			trusted = append(trusted, netip.PrefixFrom(addr, addr.BitLen()))
		} else {
			log.Printf("warning: ignoring TRUSTED_PROXIES entry %q: not an IP or CIDR", entry)
		}
	}
	isTrusted := func(peer string) bool {
		addr, err := netip.ParseAddr(peer)
		if err != nil {
			return false
		}
		addr = addr.Unmap()
		for _, prefix := range trusted {
			if prefix.Contains(addr) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := peerIP(r)
			if forwarded := strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip")); forwarded != "" && isTrusted(ip) {
				ip = forwarded
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip)))
		})
	}
}

// accessLogOutput is where accessLog writes.
var accessLogOutput io.Writer = os.Stdout

// accessLog prints one console line per request in the shape of Fiber's
// default logger, so `docker logs` reads the same.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		defer func() {
			_, _ = fmt.Fprintf(accessLogOutput, "[%s] %s %d - %v %s %s\n",
				started.Format("15:04:05"), ClientIP(r), recorder.statusCode(),
				time.Since(started).Round(time.Microsecond), r.Method, r.URL.Path)
		}()
		next.ServeHTTP(recorder, r)
	})
}

// recoverPanic turns a panic into the 500 Fiber's recover middleware sent:
// the panic value as plain text, with the stack on stderr. It sits inside
// httplog, which then logs the 500 with the panic as its cause.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}
			// Not "panic: ": the panic was recovered.
			fmt.Fprintf(os.Stderr, "recovered panic: %v\n\n%s\n", recovered, debug.Stack())
			err, ok := recovered.(error)
			if !ok {
				err = fmt.Errorf("%v", recovered)
			}
			httplog.SetError(r, err)
			writeText(w, http.StatusInternalServerError, err.Error())
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers the status a handler sent. Unwrap lets
// http.ResponseController reach Flush and Hijack on the writer beneath.
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

func (w *statusRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

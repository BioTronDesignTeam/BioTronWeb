package server

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// writeJSON answers status with body as JSON.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		httplog.SetError(r, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

// cors allows the one frontend origin with credentials, for GET and POST with
// Content-Type and X-Requested-With.
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

			// A preflight is answered here, before any route or gate; a
			// refused origin gets no Allow-Origin.
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

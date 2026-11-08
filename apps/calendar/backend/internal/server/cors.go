package server

import (
	"net/http"
	"strings"
)

// cors echoes an Origin back only when it is on the list, and answers every
// preflight itself with 204, so a preflight never reaches a route or a gate.
type cors struct {
	origins     map[string]bool
	credentials bool
	methods     string
	headers     string
}

func newCORS(origins []string, credentials bool, methods, headers []string) *cors {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))] = true
	}
	return &cors{
		origins:     allowed,
		credentials: credentials,
		methods:     strings.Join(methods, ", "),
		headers:     strings.Join(headers, ", "),
	}
}

func (c *cors) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Origin")
		origin := strings.ToLower(raw)
		// Not a CORS request, or an OPTIONS that is not a preflight: the route
		// answers it as it would anything else.
		if origin == "" || (r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") == "") {
			w.Header().Add("Vary", "Origin")
			next.ServeHTTP(w, r)
			return
		}
		allowOrigin := ""
		if c.origins[origin] {
			allowOrigin = raw
		}
		if r.Method != http.MethodOptions {
			w.Header().Add("Vary", "Origin")
			c.allowOrigin(w.Header(), allowOrigin)
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Origin")
		c.allowOrigin(w.Header(), allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", c.methods)
		w.Header().Set("Access-Control-Allow-Headers", c.headers)
		w.WriteHeader(http.StatusNoContent)
	})
}

func (c *cors) allowOrigin(header http.Header, origin string) {
	if origin == "" {
		return
	}
	header.Set("Access-Control-Allow-Origin", origin)
	if c.credentials {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
}

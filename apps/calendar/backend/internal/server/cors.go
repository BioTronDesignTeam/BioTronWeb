package server

import (
	"net/http"
	"strings"
)

// cors is the CORS policy the Fiber cors middleware applied, header for
// header, so browsers see exactly what they saw before: the origin echoed back
// only when it is on the list, a 204 for every preflight even from a stranger,
// and the same Vary lines.
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
			addVary(w.Header(), "Origin")
			next.ServeHTTP(w, r)
			return
		}
		allowOrigin := ""
		if c.origins[origin] {
			allowOrigin = raw
		}
		if r.Method != http.MethodOptions {
			addVary(w.Header(), "Origin")
			c.allowOrigin(w.Header(), allowOrigin)
			next.ServeHTTP(w, r)
			return
		}
		addVary(w.Header(), "Access-Control-Request-Method", "Access-Control-Request-Headers", "Origin")
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

// addVary adds each field to a single Vary line once, which is how Fiber kept
// it when a request passed through two CORS layers.
func addVary(header http.Header, fields ...string) {
	values := []string{}
	if existing := header.Get("Vary"); existing != "" {
		values = strings.Split(existing, ", ")
	}
	for _, field := range fields {
		present := false
		for _, value := range values {
			if strings.EqualFold(value, field) {
				present = true
				break
			}
		}
		if !present {
			values = append(values, field)
		}
	}
	header.Set("Vary", strings.Join(values, ", "))
}

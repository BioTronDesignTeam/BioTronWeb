package server

import (
	"net/http"
	"net/url"
	"strings"
)

const (
	allowMethods = "GET, POST, DELETE, PATCH, OPTIONS"
	allowHeaders = "Content-Type, X-Requested-With"
)

// siblingRoutes are the requests the other tools' frontends make to Auth with
// the session cookie: who is signed in, a permission check, both at once,
// sign-out, and Exo's guest sign-in. Nothing else of Auth answers them, so script running
// on a sibling hostname, such as through a flaw in one tool, can never grant,
// ban, rotate a key, or read the roster with a staff member's cookie.
var siblingRoutes = map[string]bool{
	"GET /auth/me":      true,
	"GET /v1/check":     true,
	"GET /v1/session":   true,
	"POST /auth/logout": true,
	"POST /auth/guest":  true,
}

// cors answers credentialed cross-origin requests from the exact origins
// listed, and no others. Auth's own frontend, frontendURL, may call every
// route; every other listed origin only siblingRoutes. Every response varies
// on Origin, so a cache never hands one origin's answer to another. A
// preflight is answered here with 204 and never reaches a route.
func cors(frontendURL string, allowedOrigins []string) middleware {
	own := originOf(frontendURL)
	siblings := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		siblings[originOf(origin)] = true
	}
	allows := func(origin, method, path string) bool {
		origin = strings.ToLower(origin)
		if own != "" && origin == own {
			return true
		}
		return siblings[origin] && siblingRoutes[method+" "+path]
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			origin := r.Header.Get("Origin")
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if origin == "" || (r.Method == http.MethodOptions && !preflight) {
				header.Add("Vary", "Origin")
				next.ServeHTTP(w, r)
				return
			}
			if !preflight {
				header.Add("Vary", "Origin")
				if allows(origin, r.Method, r.URL.Path) {
					header.Set("Access-Control-Allow-Origin", origin)
					header.Set("Access-Control-Allow-Credentials", "true")
				}
				next.ServeHTTP(w, r)
				return
			}
			header.Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Origin")
			if allows(origin, r.Header.Get("Access-Control-Request-Method"), r.URL.Path) {
				header.Set("Access-Control-Allow-Origin", origin)
				header.Set("Access-Control-Allow-Credentials", "true")
			}
			header.Set("Access-Control-Allow-Methods", allowMethods)
			header.Set("Access-Control-Allow-Headers", allowHeaders)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// originOf returns the lower-case scheme://host of a URL, or "" for one that
// has neither.
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

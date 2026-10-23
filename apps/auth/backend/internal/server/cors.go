package server

import (
	"net/http"
	"strings"
)

const (
	allowMethods = "GET, POST, DELETE, PATCH, OPTIONS"
	allowHeaders = "Content-Type, X-Requested-With"
)

// cors answers credentialed cross-origin requests from the exact origins
// listed, and no others. Every response varies on Origin, so a cache never
// hands one origin's answer to another. A preflight is answered here with
// 204 and never reaches a route.
func cors(allowedOrigins []string) middleware {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))] = true
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
			allow := allowed[strings.ToLower(origin)]
			if !preflight {
				header.Add("Vary", "Origin")
				if allow {
					header.Set("Access-Control-Allow-Origin", origin)
					header.Set("Access-Control-Allow-Credentials", "true")
				}
				next.ServeHTTP(w, r)
				return
			}
			header.Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Origin")
			if allow {
				header.Set("Access-Control-Allow-Origin", origin)
				header.Set("Access-Control-Allow-Credentials", "true")
			}
			header.Set("Access-Control-Allow-Methods", allowMethods)
			header.Set("Access-Control-Allow-Headers", allowHeaders)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

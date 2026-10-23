package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// decisionKey is where Require stashes the decision it made, so a handler
// behind the gate can read it without asking OAuthManager a second time.
type decisionKey struct{}

// Require builds middleware that refuses the request unless the caller's
// session carries permission on Exo.
//
// Wrap the whole subtree, not individual handlers, so that adding a route
// under an already-gated prefix cannot forget the check:
//
//	live := http.NewServeMux()
//	live.HandleFunc("GET /v1/live/stream", h.Stream)
//	mux.Handle("/v1/live/", authz.Require(auth.PermissionLive)(live))
//
// Status mapping:
//
//	no cookie, or OAuthManager says 401  -> 401  (sign in)
//	403, or allowed:false                -> 403  (signed in, not permitted)
//	transport error, unexpected status,
//	  or OAUTH_MANAGER_URL unset         -> 503  (fail closed, never open)
func (c *Client) Require(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decision, err := c.Check(r.Context(), r.Header.Get("Cookie"), permission)
			if err != nil {
				// An authorization service Exo cannot reach is an outage, not a
				// pass. Log it — this is the one branch an operator needs to see.
				if errors.Is(err, ErrNotConfigured) {
					log.Printf("authorization unavailable: %v", err)
				} else {
					log.Printf("authorization check for %q failed: %v", permission, err)
				}
				c.Events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{
					"permission": permission,
					"error":      err.Error(),
				})
				httplog.SetError(r, err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "authorization service unavailable"})
				return
			}
			if !decision.Authenticated {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
				return
			}
			if !decision.Allowed {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "permission required", "permission": permission})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), decisionKey{}, decision)))
		})
	}
}

// DecisionFrom returns the decision Require recorded for this request.
func DecisionFrom(r *http.Request) (Decision, bool) {
	decision, ok := r.Context().Value(decisionKey{}).(Decision)
	return decision, ok
}

// writeJSON answers the way Fiber's c.JSON did: compact, no trailing newline,
// and the charset spelled out in the content type.
func writeJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

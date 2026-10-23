package server

import (
	"net/http"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

// New builds Exo's HTTP handler. authz is the permission gate every data or
// command route must sit behind; see the route table below. trustedProxies are
// the peers whose Cf-Connecting-Ip header names the real client.
func New(frontendURL string, trustedProxies []string, events httplog.Sink, authz *auth.Client) http.Handler {
	return newHandler(frontendURL, trustedProxies, events, authz, nil)
}

// newHandler is New with a hook for tests to mount extra routes inside the
// same middleware stack.
func newHandler(frontendURL string, trustedProxies []string, events httplog.Sink, authz *auth.Client, extra func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()

	// /health is public on purpose: the container healthcheck and Logger's
	// monitor poll it, and it reveals nothing.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]any{"status": "ok"})
	})

	// Exo has no data or command routes yet. When the first one lands, mount it
	// under a prefix that is already gated rather than adding the check to the
	// handler, so a second route under the same prefix cannot forget it:
	//
	//	live := http.NewServeMux()
	//	live.HandleFunc("GET /v1/live/stream", h.Stream)
	//	mux.Handle("/v1/live/", authz.Require(auth.PermissionLive)(live))
	//
	//	hist := http.NewServeMux()
	//	hist.HandleFunc("GET /v1/historical/sessions", h.Sessions)
	//	mux.Handle("/v1/historical/", authz.Require(auth.PermissionHistorical)(hist))
	//
	//	// Commands mutate hardware, so they also need the X-Requested-With
	//	// CSRF guard the other products apply to every mutation.
	//	cmd := http.NewServeMux()
	//	cmd.HandleFunc("POST /v1/commands/stop", h.Stop)
	//	mux.Handle("/v1/commands/", requireXHR(authz.Require(auth.PermissionCommands)(cmd)))
	//
	// A daily guest key answers allowed:true for live and historical and
	// allowed:false for commands, so the AGENTS.md guest invariant holds as
	// soon as the call exists.

	if extra != nil {
		extra(mux)
	}

	return chain(fallback(mux),
		// Outermost first. The client IP is resolved before anything logs it;
		// the console line wraps httplog so both see the same final status;
		// recover sits inside httplog so a panic is logged as the 500 it became.
		clientIP(trustedProxies),
		accessLog,
		httplog.New(events, httplog.Options{}),
		recoverPanic,
		cors(frontendURL),
		limitBody(bodyLimit),
	)
}

package server

import (
	"net/http"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/exogui/backend/internal/auth"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
)

// New builds Exo's HTTP handler. authz is the permission gate every data or
// command route sits behind; see the route table below. trustedProxies are
// the peers whose Cf-Connecting-Ip header names the real client.
func New(frontendURL string, trustedProxies []string, events httplog.Sink, authz *authclient.Client) http.Handler {
	return newHandler(frontendURL, trustedProxies, events, authz, nil)
}

// statusFields are the permissions /v1/auth/status reports, one field each.
var statusFields = []authclient.StatusField{
	{Permission: auth.PermissionLive, Field: "can_live"},
	{Permission: auth.PermissionHistorical, Field: "can_historical"},
	{Permission: auth.PermissionCommands, Field: "can_commands"},
}

// gated is where each data or command route is mounted. Every one goes
// under the prefix of the permission it needs, never checked inside a
// handler, so a second route under the prefix cannot forget the gate. The
// gate runs before the route is known, so an unknown path under a prefix
// asks for a session like a real one.
type gated struct {
	// live is /v1/live/: telemetry as it arrives. Needs `live`, which a daily
	// guest key holds.
	live *http.ServeMux
	// historical is /v1/historical/: recorded sessions. Needs `historical`,
	// which a daily guest key holds.
	historical *http.ServeMux
	// commands is /v1/commands/: anything that moves or stops the hardware.
	// Needs `commands`, which a guest never holds, and X-Requested-With on
	// every mutating request.
	commands *http.ServeMux
}

// newHandler is New with a hook for tests to mount extra routes inside the
// same middleware stack.
func newHandler(frontendURL string, trustedProxies []string, events httplog.Sink, authz *authclient.Client, extra func(*http.ServeMux, gated)) http.Handler {
	mux := http.NewServeMux()

	// /health is public on purpose: the container healthcheck and Status's
	// monitor poll it, and it reveals nothing.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]any{"status": "ok"})
	})

	// Who is signed in and which of Exo's permissions they hold, so the UI
	// can turn away a member with no Exo GUI permission.
	mux.Handle("GET /v1/auth/status", authz.StatusHandler(statusFields, nil))

	// Exo has no data or command routes yet; they go in these muxes:
	//
	//	routes.live.HandleFunc("GET /v1/live/stream", h.Stream)
	//	routes.historical.HandleFunc("GET /v1/historical/sessions", h.Sessions)
	//	routes.commands.HandleFunc("POST /v1/commands/stop", h.Stop)
	//
	// A member with no grant gets 403 from all three. A daily guest key
	// opens Live and Historical and gets 403 from Commands.
	routes := gated{live: http.NewServeMux(), historical: http.NewServeMux(), commands: http.NewServeMux()}
	mux.Handle("/v1/live/", authz.Require(auth.PermissionLive)(fallback(routes.live)))
	mux.Handle("/v1/historical/", authz.Require(auth.PermissionHistorical)(fallback(routes.historical)))
	mux.Handle("/v1/commands/", authz.Require(auth.PermissionCommands)(fallback(routes.commands)))

	if extra != nil {
		extra(mux, routes)
	}

	return chain(fallback(mux),
		// Outermost first. httplog wraps everything, so its status is the one
		// the client got and a panic anywhere below is still logged. Its own
		// recover answers a bare 500; recoverPanic sits inside it to answer
		// with the panic text as Fiber did, so httplog only sees the 500.
		httplog.New(events, httplog.Options{Quiet: []string{"/v1/auth/status"}}),
		clientIP(trustedProxies),
		accessLog,
		recoverPanic,
		cors(frontendURL),
		limitBody(bodyLimit),
	)
}

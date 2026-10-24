package server

import (
	"net/http"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/httpx"
)

type middleware = func(http.Handler) http.Handler

// chain runs the middlewares in order, then h.
func chain(h http.HandlerFunc, middlewares ...middleware) http.Handler {
	var out http.Handler = h
	for i := len(middlewares) - 1; i >= 0; i-- {
		out = middlewares[i](out)
	}
	return out
}

// New returns the API. trustedProxies are the addresses (or CIDR ranges)
// whose Cf-Connecting-Ip header names the client for the guest-login limit.
func New(h *auth.Handler, allowedOrigins []string, trustedProxies []string, events *logclient.Client) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	guestLimit := newLimiter(20, time.Minute, clientIP(trustedProxies))
	mux.Handle("GET /auth/github/login", chain(h.LoginRedirect))
	mux.Handle("GET /auth/github/callback", chain(h.Callback))
	mux.Handle("POST /auth/guest", chain(h.GuestLogin, guestLimit.wrap))
	mux.Handle("POST /auth/logout", chain(h.Logout, h.RequireXHR, h.RequireSession))
	mux.Handle("GET /auth/me", chain(h.Me, h.RequireSession))
	mux.Handle("GET /auth/guest-keys", chain(h.StaffProductDailyKeys, h.RequireSession, h.RequireStaff))
	mux.Handle("POST /auth/guest-keys/{app}/rotate", chain(h.RotateProductDailyKey, h.RequireXHR, h.RequireSession, h.RequireStaff))

	mux.Handle("GET /apps", chain(h.ListApps, h.RequireSession, h.RequireNotGuest))
	mux.Handle("GET /permissions", chain(h.ListPermissions, h.RequireSession, h.RequireNotGuest))
	mux.Handle("POST /apps", chain(h.CreateApp, h.RequireXHR, h.RequireSession, h.RequireSuperuser))

	mux.Handle("GET /me/grants", chain(h.MyGrants, h.RequireSession, h.RequireNotGuest))
	mux.Handle("GET /v1/check", chain(h.Check, h.RequireSession))

	mux.Handle("POST /grants", chain(h.CreateGrant, h.RequireXHR, h.RequireSession, h.RequireStaff))
	mux.Handle("DELETE /grants", chain(h.DeleteGrant, h.RequireXHR, h.RequireSession, h.RequireStaff))

	// Every /org route is staff-only, and the session and staff checks run
	// ahead of the CSRF guard, so an anonymous caller is told it is
	// unauthenticated before anything else.
	org := func(handler http.HandlerFunc, more ...middleware) http.Handler {
		return chain(handler, append([]middleware{h.RequireSession, h.RequireStaff}, more...)...)
	}
	mux.Handle("GET /org/members", org(h.ListOrgMembers))
	mux.Handle("GET /org/members/{id}/grants", org(h.MemberGrants))
	mux.Handle("POST /org/members/{id}/ban", org(h.BanMember, h.RequireXHR))
	mux.Handle("POST /org/members/{id}/unban", org(h.UnbanMember, h.RequireXHR))
	mux.Handle("PATCH /org/members/{id}/manager", org(h.SetManager, h.RequireXHR, h.RequireSuperuser))

	// httplog outermost, so the status it logs is the one the client got and
	// a panic anywhere inside is logged as the 500 it became. The two quiet
	// paths are polled by every tool on every page load. They are logged only
	// when they fail, so the log stays a record of what happened rather than
	// a tally of who is still signed in.
	logged := httplog.New(events, httplog.Options{Quiet: []string{"/v1/check", "/auth/me"}})
	return logged(cors(allowedOrigins)(mux))
}

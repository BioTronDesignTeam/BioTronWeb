// Package server is Sprinter's admin API. Everything below /v1/admin edits
// what the bot does: who may run a command, and which Calendar scopes it
// announces. There are no public routes.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// AppID is Sprinter's application id in Auth. PermissionAdmin is the one
// permission it declares: Sprinter has no public routes and no reader tier.
const (
	AppID           = "sprinter"
	PermissionAdmin = "admin"
)

// Store is the part of the database this package uses. Threads and
// transcripts are the bot's, not the admin API's, so they are absent here.
type Store interface {
	Ping(ctx context.Context) error

	ListGuards(ctx context.Context) ([]store.Guard, error)
	GetGuard(ctx context.Context, subject string) (store.Guard, error)
	UpsertGuard(ctx context.Context, guard store.Guard) (store.Guard, error)
	DeleteGuard(ctx context.Context, subject string) error

	ListAutomations(ctx context.Context) ([]store.Automation, error)
	GetAutomation(ctx context.Context, id string) (store.Automation, error)
	CreateAutomation(ctx context.Context, automation store.Automation) (store.Automation, error)
	UpdateAutomation(ctx context.Context, automation store.Automation) (store.Automation, error)
	DeleteAutomation(ctx context.Context, id string) error
}

type Handler struct {
	store  Store
	config config.Config
	// events carries the domain events an operator's edit produces. A nil or
	// token-less client is a no-op, so the handlers call it unconditionally.
	events *logclient.Client
	// discordConnected is read on each /health, because the gateway session
	// comes and goes while the process stays up.
	discordConnected func() bool
}

// bodyLimit caps a request body. The largest real body is an automation,
// well under a kilobyte.
const bodyLimit = 256 * 1024

// New returns the whole API as one handler. The routes, statuses, headers and
// bodies are the ones the Fiber version answered, quirks included, so the
// admin page and anything probing the API see no change. authz asks Auth
// about Sprinter's sessions; New adds the error format and events to it.
func New(cfg config.Config, sprinterStore Store, authz *authclient.Client, events *logclient.Client, discordConnected func() bool) http.Handler {
	if discordConnected == nil {
		discordConnected = func() bool { return false }
	}
	handler := &Handler{
		store: sprinterStore, config: cfg,
		events: events, discordConnected: discordConnected,
	}
	authz = authz.With(authclient.WithEvents(events), authclient.WithErrorWriter(writeRefusal))
	adminCORS := newCORS(cfg.AllowedOrigins())

	admin := http.NewServeMux()
	admin.Handle("GET /v1/admin/guards", handle(handler.listGuards))
	admin.Handle("/v1/admin/guards", adminMethods("GET, HEAD, OPTIONS"))
	admin.Handle("GET /v1/admin/guards/{subject}", handle(handler.getGuard))
	admin.Handle("PUT /v1/admin/guards/{subject}", handle(handler.putGuard))
	admin.Handle("DELETE /v1/admin/guards/{subject}", handle(handler.deleteGuard))
	admin.Handle("/v1/admin/guards/{subject}", adminMethods("GET, HEAD, PUT, DELETE, OPTIONS"))
	admin.Handle("GET /v1/admin/automations", handle(handler.listAutomations))
	admin.Handle("POST /v1/admin/automations", handle(handler.createAutomation))
	admin.Handle("/v1/admin/automations", adminMethods("GET, HEAD, POST, OPTIONS"))
	admin.Handle("GET /v1/admin/automations/{id}", handle(handler.getAutomation))
	admin.Handle("PATCH /v1/admin/automations/{id}", handle(handler.patchAutomation))
	admin.Handle("DELETE /v1/admin/automations/{id}", handle(handler.deleteAutomation))
	admin.Handle("/v1/admin/automations/{id}", adminMethods("GET, HEAD, DELETE, OPTIONS, PATCH"))
	// Any other path below /v1/admin still has the preflight route, so it is
	// a wrong method rather than a missing page.
	admin.Handle("/", adminMethods("OPTIONS"))
	// The gate runs before the route is known, so an unknown admin path asks
	// for a session like a real one.
	//
	//	no cookie, or Auth refuses the session -> 401  (sign in)
	//	signed in without sprinter/admin       -> 403
	//	a write without X-Requested-With       -> 403
	//	Auth unreachable or unreadable         -> 503  (fail closed, never open)
	gated := adminCORS(authz.Require(PermissionAdmin)(admin))

	// What the page needs to draw itself: who is signed in, whether they may
	// change anything, and whether the bot runs here at all, because with
	// Discord off a guard or automation is saved but nothing acts on it.
	authStatus := authz.StatusHandler(
		[]authclient.StatusField{{Permission: PermissionAdmin, Field: "can_admin"}},
		func(*http.Request) map[string]any { return map[string]any{"discord_enabled": cfg.DiscordEnabled()} },
	)

	mux := http.NewServeMux()
	mux.Handle("GET /health", handle(handler.health))
	mux.Handle("/health", methods("GET, HEAD"))
	mux.Handle("GET /v1/auth/status", adminCORS(authStatus))
	// A preflight is answered inside adminCORS; any other OPTIONS falls
	// through to nothing.
	mux.Handle("OPTIONS /v1/auth/status", adminCORS(handle(notFound)))
	mux.Handle("/v1/auth/status", methods("GET, HEAD, OPTIONS"))
	mux.Handle("/v1/admin", gated)
	mux.Handle("/v1/admin/", gated)
	mux.Handle("/", handle(notFound))

	// httplog sits outermost so its status is the one the client received,
	// and it reports a panic as the 500 it became. The console line comes
	// next, then the recovery that turns a panic into the JSON error body.
	return httplog.New(events, httplog.Options{Quiet: []string{"/v1/auth/status"}})(
		accessLog(cfg.TrustedProxies)(
			recoverJSON(
				limitBody(bodyLimit)(
					normalizePath(mux)))))
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	database := "ok"
	status := http.StatusOK
	if err := h.store.Ping(ctx); err != nil {
		database, status = "unavailable", http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"service":           "sprinter",
		"status":            map[bool]string{true: "ok", false: "unavailable"}[status == http.StatusOK],
		"database":          database,
		"discord_enabled":   h.config.DiscordEnabled(),
		"discord_connected": h.discordConnected(),
	})
	return nil
}

// writeRefusal answers a refusal of the gate with the error body every other
// failure here gets, so the page reads one format.
func writeRefusal(w http.ResponseWriter, r *http.Request, refusal authclient.Refusal) {
	switch refusal.Status {
	case http.StatusUnauthorized:
		writeError(w, r, errUnauthorized)
	case http.StatusForbidden:
		writeError(w, r, errForbidden)
	default:
		writeError(w, r, errServiceUnavailable.because(refusal.Err))
	}
}

// adminEvent reports one admin change at Info, naming the operator behind it.
// Call it only after the store call succeeded, so the log holds what happened
// rather than what was attempted.
func (h *Handler) adminEvent(r *http.Request, message string, payload map[string]any) {
	if operator := authclient.OperatorFrom(r); operator != nil {
		payload["actor_id"] = operator.GitHubID
		payload["actor_login"] = operator.Login
	}
	h.events.LogAsync(logclient.Info, message, payload)
}

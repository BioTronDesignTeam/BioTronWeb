// Package server is Sprinter's admin API. Everything below /v1/admin edits
// what the bot does: who may run a command, and which Calendar scopes it
// announces. There are no public routes.
package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/auth"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// Authorizer is the part of the Auth client this package uses. It is an
// interface so the gate's 401/403/503 mapping can be tested without a server.
type Authorizer interface {
	Status(ctx context.Context, cookie string) (auth.Status, error)
}

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
	auth   Authorizer
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
// admin page and anything probing the API see no change.
func New(cfg config.Config, sprinterStore Store, authClient Authorizer, events *logclient.Client, discordConnected func() bool) http.Handler {
	if discordConnected == nil {
		discordConnected = func() bool { return false }
	}
	handler := &Handler{
		store: sprinterStore, auth: authClient, config: cfg,
		events: events, discordConnected: discordConnected,
	}
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
	gated := adminCORS(handle(func(w http.ResponseWriter, r *http.Request) error {
		r, err := handler.requireAdmin(r)
		if err != nil {
			return err
		}
		admin.ServeHTTP(w, r)
		return nil
	}))

	mux := http.NewServeMux()
	mux.Handle("GET /health", handle(handler.health))
	mux.Handle("/health", methods("GET, HEAD"))
	mux.Handle("GET /v1/auth/status", adminCORS(handle(handler.authStatus)))
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

// statusBody is what /v1/auth/status answers. The page needs to know whether
// the bot runs here at all, because with Discord off a guard or automation is
// saved but nothing acts on it, and an operator should not be left guessing.
type statusBody struct {
	auth.Status
	DiscordEnabled bool `json:"discord_enabled"`
}

func (h *Handler) authStatus(w http.ResponseWriter, r *http.Request) error {
	status, err := h.auth.Status(r.Context(), r.Header.Get("Cookie"))
	if err != nil {
		log.Printf("auth status: %v", err)
		h.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
		return errServiceUnavailable.because(err)
	}
	writeJSON(w, http.StatusOK, statusBody{Status: status, DiscordEnabled: h.config.DiscordEnabled()})
	return nil
}

// requireAdmin is the whole gate. It asks Auth for the session rather than the
// permission alone, so every admin event that follows can name the operator
// who caused it. It returns the request carrying that operator.
//
//	no cookie, or Auth says 401       -> 401  (sign in)
//	403, or allowed:false             -> 403  (signed in, not permitted)
//	transport error, unexpected status -> 503  (fail closed, never open)
func (h *Handler) requireAdmin(r *http.Request) (*http.Request, error) {
	// A mutating request without this header cannot come from a cross-site
	// form post, because a form cannot set it and a browser preflights it.
	if mutating(r.Method) && r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
		return r, errForbidden
	}
	status, err := h.auth.Status(r.Context(), r.Header.Get("Cookie"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			return r, errUnauthorized
		case errors.Is(err, auth.ErrForbidden):
			return r, errForbidden
		default:
			log.Printf("authorization check: %v", err)
			h.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
			return r, errServiceUnavailable
		}
	}
	// Status answers an absent or rejected session with an empty operator and
	// no error, which is the 401.
	if status.Operator == nil {
		return r, errUnauthorized
	}
	if !status.CanAdmin {
		return r, errForbidden
	}
	httplog.SetActor(r, status.Operator.Login)
	return r.WithContext(context.WithValue(r.Context(), operatorKey, status.Operator)), nil
}

type contextKey string

// operatorKey holds the operator requireAdmin resolved. The key has its own
// type so nothing else in the request can collide with it.
const operatorKey contextKey = "sprinter.operator"

func operatorFrom(r *http.Request) *auth.Operator {
	operator, _ := r.Context().Value(operatorKey).(*auth.Operator)
	return operator
}

// adminEvent reports one admin change at Info, naming the operator behind it.
// Call it only after the store call succeeded, so the log holds what happened
// rather than what was attempted.
func (h *Handler) adminEvent(r *http.Request, message string, payload map[string]any) {
	if operator := operatorFrom(r); operator != nil {
		payload["actor_id"] = operator.GitHubID
		payload["actor_login"] = operator.Login
	}
	h.events.LogAsync(logclient.Info, message, payload)
}

func mutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

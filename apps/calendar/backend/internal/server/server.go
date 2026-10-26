package server

import (
	"context"
	"net/http"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

// AppID is Calendar's application id in Auth. PermissionWrite is the one
// permission it declares: every editor change needs it, and public reads need
// nothing.
const (
	AppID           = "calendar"
	PermissionWrite = "write"
)

type Handler struct {
	store  *store.Store
	config config.Config
	// events carries the domain events an operator's edit produces. A nil or
	// token-less client is a no-op, so the handlers call it unconditionally.
	events   *logclient.Client
	location *time.Location
	// now is injectable so the reads whose answer depends on the clock can be
	// pinned against a fixture instead of whatever today happens to be.
	now func() time.Time
}

// New returns the whole API as one http.Handler. The routes, statuses,
// headers, and bodies are the ones the Fiber app served, because the
// frontends, the site, and every subscribed calendar depend on them.
func New(cfg config.Config, calendarStore *store.Store, authz *authclient.Client, events *logclient.Client, location *time.Location) http.Handler {
	h := &Handler{
		store: calendarStore, config: cfg,
		events: events, location: location, now: time.Now,
	}
	authz = withCalendarErrors(authz, events)
	publicCORS := newCORS(cfg.PublicAllowedOrigins(), false,
		[]string{http.MethodGet, http.MethodHead, http.MethodOptions},
		[]string{"Content-Type"})
	adminCORS := newCORS(cfg.AdminAllowedOrigins(), true,
		[]string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		[]string{"Content-Type", "X-Requested-With"})

	public := func(handle handlerFunc) http.Handler { return publicCORS.wrap(h.serve(handle)) }
	// An OPTIONS request that is not a preflight passes through CORS to
	// nothing, which Fiber answered with a 404.
	publicOptions := publicCORS.wrap(http.HandlerFunc(notFound))

	mux := http.NewServeMux()
	mux.Handle("GET /health", h.serve(h.health))
	mux.Handle("GET /v1/auth/status", adminCORS.wrap(authz.StatusHandler(
		[]authclient.StatusField{{Permission: PermissionWrite, Field: "can_write"}}, nil)))
	mux.Handle("OPTIONS /v1/auth/status", adminCORS.wrap(http.HandlerFunc(notFound)))
	mux.Handle("GET /v1/scopes", public(h.listScopes))
	mux.Handle("OPTIONS /v1/scopes", publicOptions)
	mux.Handle("GET /v1/events", public(h.listOccurrences))
	mux.Handle("OPTIONS /v1/events", publicOptions)
	mux.Handle("GET /v1/events/upcoming", public(h.listUpcoming))
	mux.Handle("OPTIONS /v1/events/upcoming", publicOptions)
	mux.Handle("GET /v1/feeds/all.ics", public(h.allFeed))
	mux.Handle("GET /v1/feeds/scopes/{file}", feedFile(public(h.scopeFeed)))
	mux.Handle("OPTIONS /v1/feeds", publicOptions)
	mux.Handle("OPTIONS /v1/feeds/", publicOptions)

	// Everything under /v1/admin passes through the admin CORS policy and the
	// calendar/write gate before it is routed, unknown paths included, as it did
	// under Fiber's group middleware.
	admin := http.NewServeMux()
	admin.Handle("GET /v1/admin/scopes", h.serve(h.listAdminScopes))
	admin.Handle("POST /v1/admin/scopes", h.serve(h.createScope))
	admin.Handle("PATCH /v1/admin/scopes/{id}", h.serve(h.renameScope))
	admin.Handle("POST /v1/admin/scopes/{id}/archive", h.serve(h.archiveScope))
	admin.Handle("POST /v1/admin/scopes/{id}/restore", h.serve(h.restoreScope))
	admin.Handle("DELETE /v1/admin/scopes/{id}", h.serve(h.deleteScope))
	admin.Handle("GET /v1/admin/events", h.serve(h.listAdminEvents))
	admin.Handle("POST /v1/admin/events", h.serve(h.createEvent))
	admin.Handle("PATCH /v1/admin/events/{id}", h.serve(h.updateEvent))
	admin.Handle("POST /v1/admin/events/{id}/publish", h.serve(h.publishEvent))
	admin.Handle("POST /v1/admin/events/{id}/cancel", h.serve(h.cancelEvent))
	admin.Handle("DELETE /v1/admin/events/{id}", h.serve(h.deleteEvent))
	admin.Handle("PUT /v1/admin/events/{id}/occurrences", h.serve(h.upsertOccurrence))
	admin.Handle("DELETE /v1/admin/events/{id}/occurrences", h.serve(h.deleteOccurrenceOverride))
	admin.Handle("OPTIONS /v1/admin", http.HandlerFunc(notFound))
	admin.Handle("OPTIONS /v1/admin/", http.HandlerFunc(notFound))
	adminRoutes := adminCORS.wrap(authz.Require(PermissionWrite)(routeJSON(admin)))

	routes := routeJSON(mux)
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAdminPath(r.URL.Path) {
			adminRoutes.ServeHTTP(w, r)
			return
		}
		routes.ServeHTTP(w, r)
	})
	// httplog is outermost so the status it logs is the one the client got,
	// and recoverJSON inside it so a panic is logged as the 500 it became.
	return httplog.New(events, httplog.Options{})(recoverJSON(fiberPaths(app)))
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		return writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"service": "biotron-calendar", "status": "unavailable",
		})
	}
	return writeJSON(w, http.StatusOK, map[string]any{"service": "biotron-calendar", "status": "ok"})
}

// withCalendarErrors returns authz answering its refusals with Calendar's own
// error body, {"error":"unauthorized"} and the like, which the admin page
// shows, and reporting an Auth outage as an event. The public routes never
// reach it: they call nothing.
func withCalendarErrors(authz *authclient.Client, events *logclient.Client) *authclient.Client {
	return authz.With(authclient.WithEvents(events), authclient.WithErrorWriter(
		func(w http.ResponseWriter, r *http.Request, refusal authclient.Refusal) {
			var err *httpError
			switch refusal.Status {
			case http.StatusUnauthorized:
				err = errUnauthorized
			case http.StatusForbidden:
				err = errForbidden
			default:
				// The request event keeps the cause authclient recorded.
				writeError(w, errServiceUnavailable)
				return
			}
			httplog.SetError(r, err)
			writeError(w, err)
		}))
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

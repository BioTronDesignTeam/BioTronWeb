package server

import (
	"context"
	"net/http"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/httplog"
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

// New returns the whole API as one http.Handler. The public routes answer
// any origin on the public list without credentials; the editor routes, and
// the status the editor page reads, answer only the editor origins.
func New(cfg config.Config, calendarStore *store.Store, authz *authclient.Client, events *logclient.Client, location *time.Location) http.Handler {
	h := &Handler{
		store: calendarStore, config: cfg,
		events: events, location: location, now: time.Now,
	}
	authz = authz.With(authclient.WithEvents(events))
	publicCORS := newCORS(cfg.PublicAllowedOrigins(), false,
		[]string{http.MethodGet, http.MethodHead, http.MethodOptions},
		[]string{"Content-Type"})
	adminCORS := newCORS(cfg.AdminAllowedOrigins(), true,
		[]string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		[]string{"Content-Type", "X-Requested-With"})

	public := http.NewServeMux()
	public.Handle("GET /v1/scopes", h.serve(h.listScopes))
	public.Handle("GET /v1/events", h.serve(h.listOccurrences))
	public.Handle("GET /v1/events/upcoming", h.serve(h.listUpcoming))
	public.Handle("GET /v1/feeds/all.ics", h.serve(h.allFeed))
	public.Handle("GET /v1/feeds/scopes/{file}", h.serve(h.scopeFeed))

	status := http.NewServeMux()
	status.Handle("GET /v1/auth/status", authz.StatusHandler(
		[]authclient.StatusField{{Permission: PermissionWrite, Field: "can_write"}}, nil))

	// Every editor route needs calendar/write. The gate wraps the whole
	// prefix, so a route added under it cannot forget the check.
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

	mux := http.NewServeMux()
	mux.Handle("GET /health", h.serve(h.health))
	mux.Handle("/v1/", publicCORS.wrap(public))
	mux.Handle("/v1/auth/status", adminCORS.wrap(status))
	mux.Handle("/v1/admin/", adminCORS.wrap(authz.Require(PermissionWrite)(admin)))
	// httplog is outermost so the status it logs is the one the client got,
	// and a panic is logged as the 500 it became.
	return httplog.New(events, httplog.Options{})(mux)
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

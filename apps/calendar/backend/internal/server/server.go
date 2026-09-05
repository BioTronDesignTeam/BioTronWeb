package server

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/fiberlog"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/auth"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/model"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

type Handler struct {
	store  *store.Store
	auth   *auth.Client
	config config.Config
	// events carries the domain events an operator's edit produces. A nil or
	// token-less client is a no-op, so the handlers call it unconditionally.
	events   *logclient.Client
	location *time.Location
	// now is injectable so the reads whose answer depends on the clock can be
	// pinned against a fixture instead of whatever today happens to be.
	now func() time.Time
}

func New(cfg config.Config, calendarStore *store.Store, authClient *auth.Client, events *logclient.Client, location *time.Location) *fiber.App {
	handler := &Handler{
		store: calendarStore, auth: authClient, config: cfg,
		events: events, location: location, now: time.Now,
	}
	app := fiber.New(fiber.Config{
		BodyLimit:   256 * 1024,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
		// Per-request client identity comes from Cloudflare, but only when the
		// peer is one of our own proxies. TrustProxy plus an explicit Proxies
		// allow-list is Fiber v3's spelling of v2's EnableTrustedProxyCheck and
		// TrustedProxies; without both, c.IP() would take a spoofable header
		// from any caller.
		TrustProxy:       true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: cfg.TrustedProxies},
		ProxyHeader:      "Cf-Connecting-Ip",
		ErrorHandler:     jsonErrorHandler,
	})

	// Fiber's console logger first: it runs the error handler itself and
	// returns nil, so fiberlog must sit inside it to see a returned error,
	// and recover inside fiberlog so a panic is logged as the 500 it became.
	app.Use(logger.New())
	app.Use(fiberlog.New(events, fiberlog.Options{}))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	publicCORS := cors.New(cors.Config{
		AllowOrigins: cfg.PublicAllowedOrigins(),
		AllowMethods: []string{fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions},
		AllowHeaders: []string{fiber.HeaderContentType},
	})
	adminCORS := cors.New(cors.Config{
		AllowOrigins:     cfg.AdminAllowedOrigins(),
		AllowCredentials: true,
		AllowMethods: []string{
			fiber.MethodGet, fiber.MethodPost, fiber.MethodPut,
			fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions,
		},
		AllowHeaders: []string{fiber.HeaderContentType, "X-Requested-With"},
	})

	app.Get("/health", handler.health)
	app.Get("/v1/auth/status", adminCORS, handler.authStatus)
	app.Get("/v1/scopes", publicCORS, handler.listScopes)
	app.Get("/v1/events", publicCORS, handler.listOccurrences)
	app.Get("/v1/events/upcoming", publicCORS, handler.listUpcoming)
	app.Get("/v1/feeds/all.ics", publicCORS, handler.allFeed)
	app.Get("/v1/feeds/scopes/:id.ics", publicCORS, handler.scopeFeed)
	app.Options("/v1/auth/status", adminCORS)
	app.Options("/v1/scopes", publicCORS)
	app.Options("/v1/events", publicCORS)
	app.Options("/v1/events/upcoming", publicCORS)
	app.Options("/v1/feeds/*", publicCORS)
	app.Options("/v1/admin/*", adminCORS)

	admin := app.Group("/v1/admin", adminCORS, handler.requireWrite)
	admin.Get("/scopes", handler.listAdminScopes)
	admin.Post("/scopes", handler.createScope)
	admin.Patch("/scopes/:id", handler.renameScope)
	admin.Post("/scopes/:id/archive", handler.archiveScope)
	admin.Post("/scopes/:id/restore", handler.restoreScope)
	admin.Delete("/scopes/:id", handler.deleteScope)
	admin.Get("/events", handler.listAdminEvents)
	admin.Post("/events", handler.createEvent)
	admin.Patch("/events/:id", handler.updateEvent)
	admin.Post("/events/:id/publish", handler.publishEvent)
	admin.Post("/events/:id/cancel", handler.cancelEvent)
	admin.Delete("/events/:id", handler.deleteEvent)
	admin.Put("/events/:id/occurrences", handler.upsertOccurrence)
	admin.Delete("/events/:id/occurrences", handler.deleteOccurrenceOverride)

	return app
}

func (h *Handler) health(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"service": "biotron-calendar", "status": "unavailable",
		})
	}
	return c.JSON(fiber.Map{"service": "biotron-calendar", "status": "ok"})
}

func (h *Handler) authStatus(c fiber.Ctx) error {
	status, err := h.auth.Status(c.Context(), c.Get("Cookie"))
	if err != nil {
		log.Printf("auth status: %v", err)
		h.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
		return fiber.ErrServiceUnavailable
	}
	return c.JSON(status)
}

// requireWrite asks Auth for the whole session rather than the permission
// alone, so every admin event that follows can name the operator who caused
// it. That is one more call to Auth per admin request, which admin traffic can
// afford; the public routes still call nothing.
func (h *Handler) requireWrite(c fiber.Ctx) error {
	if requiresRequestHeader(c.Method()) && c.Get("X-Requested-With") != "XMLHttpRequest" {
		return fiber.ErrForbidden
	}
	status, err := h.auth.Status(c.Context(), c.Get("Cookie"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			return fiber.ErrUnauthorized
		case errors.Is(err, auth.ErrForbidden):
			return fiber.ErrForbidden
		default:
			log.Printf("authorization check: %v", err)
			h.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
			return fiber.ErrServiceUnavailable
		}
	}
	// Status answers an absent or rejected session with an empty operator and
	// no error, which is the 401 CanWrite used to raise itself.
	if status.Operator == nil {
		return fiber.ErrUnauthorized
	}
	if !status.CanWrite {
		return fiber.ErrForbidden
	}
	c.Locals(operatorKey, status.Operator)
	fiberlog.SetActor(c, status.Operator.Login)
	return c.Next()
}

type localKey string

// operatorKey holds the operator requireWrite resolved. The key has its own
// type so nothing else in the request can collide with it.
const operatorKey localKey = "calendar.operator"

// operatorFrom returns the operator requireWrite put on the request, or nil on
// a route that does not require one.
func operatorFrom(c fiber.Ctx) *model.Operator {
	operator, _ := c.Locals(operatorKey).(*model.Operator)
	return operator
}

// adminEvent reports one admin change at Info, naming the operator behind it.
// Call it only after the store call succeeded, so the log holds what happened
// rather than what was attempted.
func (h *Handler) adminEvent(c fiber.Ctx, message string, payload map[string]any) {
	if operator := operatorFrom(c); operator != nil {
		payload["actor_id"] = operator.GitHubID
		payload["actor_login"] = operator.Login
	}
	h.events.LogAsync(logclient.Info, message, payload)
}

func requiresRequestHeader(method string) bool {
	switch method {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return false
	default:
		return true
	}
}

func jsonErrorHandler(c fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	message := "internal server error"
	var fiberError *fiber.Error
	if errors.As(err, &fiberError) {
		status = fiberError.Code
		message = strings.ToLower(fiberError.Message)
	} else if errors.Is(err, store.ErrNotFound) {
		status, message = fiber.StatusNotFound, "not found"
	} else if errors.Is(err, store.ErrConflict) {
		status, message = fiber.StatusConflict, "the requested change conflicts with existing calendar data"
	}
	if status >= 500 {
		log.Printf("request failed: %v", err)
	}
	return c.Status(status).JSON(fiber.Map{"error": message})
}

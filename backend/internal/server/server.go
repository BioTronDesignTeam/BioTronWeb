package server

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/auth"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/eventlog"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

type Handler struct {
	store    *store.Store
	auth     *auth.Client
	config   config.Config
	location *time.Location
}

func New(cfg config.Config, calendarStore *store.Store, authClient *auth.Client, events *eventlog.Client, location *time.Location) *fiber.App {
	handler := &Handler{store: calendarStore, auth: authClient, config: cfg, location: location}
	app := fiber.New(fiber.Config{
		DisableStartupMessage:   true,
		BodyLimit:               256 * 1024,
		ReadTimeout:             15 * time.Second,
		IdleTimeout:             60 * time.Second,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          cfg.TrustedProxies,
		ProxyHeader:             "Cf-Connecting-Ip",
		ErrorHandler:            jsonErrorHandler,
	})

	app.Use(logRequests(events))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(logger.New())
	publicCORS := cors.New(cors.Config{
		AllowOrigins: strings.Join(cfg.PublicAllowedOrigins(), ","),
		AllowMethods: "GET,HEAD,OPTIONS",
		AllowHeaders: "Content-Type",
	})
	adminCORS := cors.New(cors.Config{
		AllowOrigins:     strings.Join(cfg.AdminAllowedOrigins(), ","),
		AllowCredentials: true,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Content-Type,X-Requested-With",
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

func (h *Handler) health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"service": "biotron-calendar", "status": "unavailable",
		})
	}
	return c.JSON(fiber.Map{"service": "biotron-calendar", "status": "ok"})
}

func (h *Handler) authStatus(c *fiber.Ctx) error {
	status, err := h.auth.Status(c.UserContext(), c.Get("Cookie"))
	if err != nil {
		log.Printf("auth status: %v", err)
		return fiber.ErrServiceUnavailable
	}
	return c.JSON(status)
}

func (h *Handler) requireWrite(c *fiber.Ctx) error {
	if requiresRequestHeader(c.Method()) && c.Get("X-Requested-With") != "XMLHttpRequest" {
		return fiber.ErrForbidden
	}
	allowed, err := h.auth.CanWrite(c.UserContext(), c.Get("Cookie"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			return fiber.ErrUnauthorized
		case errors.Is(err, auth.ErrForbidden):
			return fiber.ErrForbidden
		default:
			log.Printf("authorization check: %v", err)
			return fiber.ErrServiceUnavailable
		}
	}
	if !allowed {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func requiresRequestHeader(method string) bool {
	switch method {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return false
	default:
		return true
	}
}

func jsonErrorHandler(c *fiber.Ctx, err error) error {
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

func logRequests(events *eventlog.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		started := time.Now()
		err := c.Next()
		if c.Path() == "/health" {
			return err
		}
		status := c.Response().StatusCode()
		if err != nil {
			status = fiber.StatusInternalServerError
			var fiberError *fiber.Error
			if errors.As(err, &fiberError) {
				status = fiberError.Code
			}
		}
		if status < 400 && c.Method() == fiber.MethodOptions {
			return err
		}
		level := eventlog.Info
		if status >= 500 {
			level = eventlog.Error
		} else if status >= 400 {
			level = eventlog.Warning
		}
		events.LogAsync(level, "HTTP request completed", map[string]any{
			"method": c.Method(), "path": c.Path(), "status": status,
			"duration_ms": time.Since(started).Milliseconds(),
		})
		return err
	}
}

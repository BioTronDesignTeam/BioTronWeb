package server

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/eventlog"
)

// New builds Exo's HTTP app. authz is the permission gate every data or command
// route must sit behind; see the route table below.
func New(frontendURL string, trustedProxies []string, events *eventlog.Client, authz *auth.Client) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage:   true,
		ReadTimeout:             15 * time.Second,
		IdleTimeout:             60 * time.Second,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          trustedProxies,
		ProxyHeader:             "Cf-Connecting-Ip",
	})

	app.Use(logRequests(events))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     frontendURL,
		AllowCredentials: true,
		AllowMethods:     "GET,POST,OPTIONS",
		AllowHeaders:     "Content-Type,X-Requested-With",
	}))

	// /health is public on purpose: the container healthcheck and Logger's
	// monitor poll it, and it reveals nothing.
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Exo has no data or command routes yet. When the first one lands, mount it
	// on a group that is already gated rather than adding the check to the
	// handler, so a second route on the same group cannot forget it:
	//
	//	live := app.Group("/v1/live", authz.Require(auth.PermissionLive))
	//	live.Get("/stream", h.Stream)
	//
	//	hist := app.Group("/v1/historical", authz.Require(auth.PermissionHistorical))
	//	hist.Get("/sessions", h.Sessions)
	//
	//	// Commands mutate hardware, so they also need the X-Requested-With
	//	// CSRF guard the other products apply to every mutation.
	//	cmd := app.Group("/v1/commands", requireXHR, authz.Require(auth.PermissionCommands))
	//	cmd.Post("/stop", h.Stop)
	//
	// A daily guest key answers allowed:true for live and historical and
	// allowed:false for commands, so the AGENTS.md guest invariant holds as
	// soon as the call exists.

	return app
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
			"method":      c.Method(),
			"path":        c.Path(),
			"status":      status,
			"duration_ms": time.Since(started).Milliseconds(),
		})
		return err
	}
}

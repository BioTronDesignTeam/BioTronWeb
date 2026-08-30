package server

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/eventlog"
)

func New(frontendURL string, trustedProxies []string, events *eventlog.Client) *fiber.App {
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

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

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

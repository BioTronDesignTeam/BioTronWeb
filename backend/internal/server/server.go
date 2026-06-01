package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

// New builds the Fiber app with CORS, logging, health, and the auth routes.
func New(h *auth.Handler, frontendURL string) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     frontendURL,
		AllowCredentials: true,
		AllowMethods:     "GET,POST,OPTIONS",
		AllowHeaders:     "Content-Type",
	}))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	a := app.Group("/auth")
	a.Get("/github/login", h.LoginRedirect)
	a.Get("/github/callback", h.Callback)
	a.Post("/logout", h.Logout)
	a.Get("/me", h.RequireSession, h.Me)

	return app
}

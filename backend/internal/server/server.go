package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
)

func New(h *auth.Handler, allowedOrigins []string, trustedProxies []string) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage:   true,
		ReadTimeout:             15 * time.Second,
		IdleTimeout:             60 * time.Second,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          trustedProxies,
		ProxyHeader:             "Cf-Connecting-Ip",
	})

	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Join(allowedOrigins, ","),
		AllowCredentials: true,
		AllowMethods:     "GET,POST,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Content-Type,X-Requested-With",
	}))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	a := app.Group("/auth")
	a.Get("/github/login", h.LoginRedirect)
	a.Get("/github/callback", h.Callback)
	a.Post("/guest", limiter.New(limiter.Config{Max: 20, Expiration: time.Minute}), h.GuestLogin)
	a.Post("/logout", h.RequireXHR, h.RequireSession, h.Logout)
	a.Get("/me", h.RequireSession, h.Me)
	a.Get("/guest-keys", h.RequireSession, h.RequireStaff, h.StaffProductDailyKeys)

	app.Get("/apps", h.RequireSession, h.ListApps)
	app.Get("/permissions", h.RequireSession, h.ListPermissions)
	app.Post("/apps", h.RequireXHR, h.RequireSession, h.RequireSuperuser, h.CreateApp)

	app.Get("/me/grants", h.RequireSession, h.MyGrants)
	app.Get("/me/requests", h.RequireSession, h.MyRequests)
	app.Get("/v1/check", h.RequireSession, h.Check)

	app.Post("/requests", h.RequireXHR, h.RequireSession, h.CreateRequest)
	app.Get("/requests/pending", h.RequireSession, h.RequireStaff, h.PendingRequests)
	app.Post("/requests/:id/approve", h.RequireXHR, h.RequireSession, h.RequireStaff, h.ApproveRequest)
	app.Post("/requests/:id/deny", h.RequireXHR, h.RequireSession, h.RequireStaff, h.DenyRequest)

	app.Post("/grants", h.RequireXHR, h.RequireSession, h.RequireStaff, h.CreateGrant)
	app.Delete("/grants", h.RequireXHR, h.RequireSession, h.RequireStaff, h.DeleteGrant)

	org := app.Group("/org", h.RequireSession, h.RequireStaff)
	org.Get("/members", h.ListOrgMembers)
	org.Get("/members/:id/grants", h.MemberGrants)
	org.Post("/members/:id/ban", h.RequireXHR, h.BanMember)
	org.Post("/members/:id/unban", h.RequireXHR, h.UnbanMember)
	org.Patch("/members/:id/manager", h.RequireXHR, h.RequireSuperuser, h.SetManager)

	return app
}

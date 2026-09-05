package server

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/BioTronDesignTeam/biotron/go/logclient"
	"github.com/BioTronDesignTeam/biotron/go/logclient/fiberlog"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
)

func New(h *auth.Handler, allowedOrigins []string, trustedProxies []string, events *logclient.Client) *fiber.App {
	app := fiber.New(fiber.Config{
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
		// Fiber v3 renamed EnableTrustedProxyCheck/TrustedProxies to
		// TrustProxy/TrustProxyConfig.Proxies. The meaning is unchanged: only
		// a request arriving from one of these addresses may set the client IP
		// through ProxyHeader, so the per-IP limiter keys on the real caller
		// instead of collapsing into one bucket for the edge.
		TrustProxy:       true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: trustedProxies},
		ProxyHeader:      "Cf-Connecting-Ip",
	})

	// Fiber's console logger first: it runs the error handler itself and
	// returns nil, so fiberlog must sit inside it to see a returned error,
	// and recover inside fiberlog so a panic is logged as the 500 it became.
	app.Use(logger.New())
	// The two quiet paths are polled by every tool on every page load. They are
	// logged only when they fail, so the log stays a record of what happened
	// rather than a tally of who is still signed in.
	app.Use(fiberlog.New(events, fiberlog.Options{Quiet: []string{"/v1/check", "/auth/me"}}))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	// v3 takes slices where v2 took comma-joined strings; the allow-list is
	// still exact-match, still credentialed, and the middleware still sets
	// Vary: Origin on every non-wildcard response.
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowCredentials: true,
		AllowMethods: []string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodDelete,
			fiber.MethodPatch,
			fiber.MethodOptions,
		},
		AllowHeaders: []string{fiber.HeaderContentType, "X-Requested-With"},
	}))

	app.Get("/health", func(c fiber.Ctx) error {
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
	app.Get("/v1/check", h.RequireSession, h.Check)

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

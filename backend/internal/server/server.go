package server

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/eventlog"
)

func New(h *auth.Handler, allowedOrigins []string, trustedProxies []string, events *eventlog.Client) *fiber.App {
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

	app.Use(logRequests(events))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(logger.New())
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

func logRequests(events *eventlog.Client) fiber.Handler {
	return func(c fiber.Ctx) error {
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
		if status < 400 && (c.Method() == fiber.MethodOptions || c.Path() == "/v1/check" || c.Path() == "/auth/me") {
			return err
		}
		level := eventlog.Info
		if status >= 500 {
			level = eventlog.Error
		} else if status >= 400 {
			level = eventlog.Warning
		}
		// Clone before handing these to LogAsync. Fiber returns method and path
		// as strings pointing into the pooled request buffer, and LogAsync
		// marshals the map on another goroutine — by which time this request
		// can be finished and its buffer refilled by a different one. Without
		// the copy a log line can report another request's path, which is both
		// wrong and a way for a path we never meant to log to surface.
		events.LogAsync(level, "HTTP request completed", map[string]any{
			"method":      strings.Clone(c.Method()),
			"path":        strings.Clone(c.Path()),
			"status":      status,
			"duration_ms": time.Since(started).Milliseconds(),
		})
		return err
	}
}

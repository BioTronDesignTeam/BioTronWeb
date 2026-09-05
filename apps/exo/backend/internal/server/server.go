package server

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/fiberlog"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

// New builds Exo's HTTP app. authz is the permission gate every data or command
// route must sit behind; see the route table below.
func New(frontendURL string, trustedProxies []string, events *logclient.Client, authz *auth.Client) *fiber.App {
	app := fiber.New(fiber.Config{
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
		// Fiber v3 renamed the v2 pair EnableTrustedProxyCheck/TrustedProxies to
		// TrustProxy plus TrustProxyConfig.Proxies. Same behaviour, same reason:
		// without it every request appears to come from the edge proxy and the
		// per-IP rate limiters all share one bucket.
		TrustProxy:       true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: trustedProxies},
		ProxyHeader:      "Cf-Connecting-Ip",
	})

	// Fiber's console logger first: it runs the error handler itself and
	// returns nil, so fiberlog must sit inside it to see a returned error,
	// and recover inside fiberlog so a panic is logged as the 500 it became.
	app.Use(logger.New())
	app.Use(fiberlog.New(events, fiberlog.Options{}))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	// v3 takes these as slices rather than comma-separated strings.
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{frontendURL},
		AllowCredentials: true,
		AllowMethods:     []string{fiber.MethodGet, fiber.MethodPost, fiber.MethodOptions},
		AllowHeaders:     []string{fiber.HeaderContentType, "X-Requested-With"},
	}))

	// /health is public on purpose: the container healthcheck and Logger's
	// monitor poll it, and it reveals nothing.
	app.Get("/health", func(c fiber.Ctx) error {
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

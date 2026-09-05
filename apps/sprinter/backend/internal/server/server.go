// Package server is Sprinter's admin API. Everything below /v1/admin edits
// what the bot does: who may run a command, and which Calendar scopes it
// announces. There are no public routes.
package server

import (
	"context"
	"encoding/json"
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
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/auth"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// Authorizer is the part of the Auth client this package uses. It is an
// interface so the gate's 401/403/503 mapping can be tested without a server.
type Authorizer interface {
	Status(ctx context.Context, cookie string) (auth.Status, error)
}

// Store is the part of the database this package uses. Threads and
// transcripts are the bot's, not the admin API's, so they are absent here.
type Store interface {
	Ping(ctx context.Context) error

	ListGuards(ctx context.Context) ([]store.Guard, error)
	GetGuard(ctx context.Context, subject string) (store.Guard, error)
	UpsertGuard(ctx context.Context, guard store.Guard) (store.Guard, error)
	DeleteGuard(ctx context.Context, subject string) error

	ListAutomations(ctx context.Context) ([]store.Automation, error)
	GetAutomation(ctx context.Context, id string) (store.Automation, error)
	CreateAutomation(ctx context.Context, automation store.Automation) (store.Automation, error)
	UpdateAutomation(ctx context.Context, automation store.Automation) (store.Automation, error)
	DeleteAutomation(ctx context.Context, id string) error
}

type Handler struct {
	store  Store
	auth   Authorizer
	config config.Config
	// events carries the domain events an operator's edit produces. A nil or
	// token-less client is a no-op, so the handlers call it unconditionally.
	events *logclient.Client
	// discordConnected is read on each /health, because the gateway session
	// comes and goes while the process stays up.
	discordConnected func() bool
}

func New(cfg config.Config, sprinterStore Store, authClient Authorizer, events *logclient.Client, discordConnected func() bool) *fiber.App {
	if discordConnected == nil {
		discordConnected = func() bool { return false }
	}
	handler := &Handler{
		store: sprinterStore, auth: authClient, config: cfg,
		events: events, discordConnected: discordConnected,
	}
	app := fiber.New(fiber.Config{
		BodyLimit:   256 * 1024,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
		// Client identity comes from Cloudflare, but only when the peer is one
		// of our own proxies. Without the allow-list, c.IP() would take a
		// spoofable header from any caller.
		TrustProxy:       true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: cfg.TrustedProxies},
		ProxyHeader:      "Cf-Connecting-Ip",
		ErrorHandler:     jsonErrorHandler,
	})

	// Fiber's console logger first: it runs the error handler itself and
	// returns nil, so fiberlog must sit inside it to see a returned error, and
	// recover inside fiberlog so a panic is logged as the 500 it became.
	app.Use(logger.New())
	app.Use(fiberlog.New(events, fiberlog.Options{Quiet: []string{"/v1/auth/status"}}))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))

	adminCORS := cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins(),
		AllowCredentials: true,
		AllowMethods: []string{
			fiber.MethodGet, fiber.MethodPost, fiber.MethodPut,
			fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions,
		},
		AllowHeaders: []string{fiber.HeaderContentType, "X-Requested-With"},
	})

	app.Get("/health", handler.health)
	app.Get("/v1/auth/status", adminCORS, handler.authStatus)
	app.Options("/v1/auth/status", adminCORS)
	app.Options("/v1/admin/*", adminCORS)

	admin := app.Group("/v1/admin", adminCORS, handler.requireAdmin)
	admin.Get("/guards", handler.listGuards)
	admin.Get("/guards/:subject", handler.getGuard)
	admin.Put("/guards/:subject", handler.putGuard)
	admin.Delete("/guards/:subject", handler.deleteGuard)
	admin.Get("/automations", handler.listAutomations)
	admin.Post("/automations", handler.createAutomation)
	admin.Get("/automations/:id", handler.getAutomation)
	admin.Patch("/automations/:id", handler.patchAutomation)
	admin.Delete("/automations/:id", handler.deleteAutomation)

	return app
}

func (h *Handler) health(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), time.Second)
	defer cancel()
	database := "ok"
	status := fiber.StatusOK
	if err := h.store.Ping(ctx); err != nil {
		database, status = "unavailable", fiber.StatusServiceUnavailable
	}
	return c.Status(status).JSON(fiber.Map{
		"service":           "sprinter",
		"status":            map[bool]string{true: "ok", false: "unavailable"}[status == fiber.StatusOK],
		"database":          database,
		"discord_connected": h.discordConnected(),
	})
}

func (h *Handler) authStatus(c fiber.Ctx) error {
	status, err := h.auth.Status(c.Context(), c.Get(fiber.HeaderCookie))
	if err != nil {
		log.Printf("auth status: %v", err)
		h.events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{"error": err.Error()})
		fiberlog.SetError(c, err)
		return fiber.ErrServiceUnavailable
	}
	return c.JSON(status)
}

// requireAdmin is the whole gate. It asks Auth for the session rather than the
// permission alone, so every admin event that follows can name the operator
// who caused it.
//
//	no cookie, or Auth says 401       -> 401  (sign in)
//	403, or allowed:false             -> 403  (signed in, not permitted)
//	transport error, unexpected status -> 503  (fail closed, never open)
func (h *Handler) requireAdmin(c fiber.Ctx) error {
	// A mutating request without this header cannot come from a cross-site
	// form post, because a form cannot set it and a browser preflights it.
	if mutating(c.Method()) && c.Get("X-Requested-With") != "XMLHttpRequest" {
		return fiber.ErrForbidden
	}
	status, err := h.auth.Status(c.Context(), c.Get(fiber.HeaderCookie))
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
	// no error, which is the 401.
	if status.Operator == nil {
		return fiber.ErrUnauthorized
	}
	if !status.CanAdmin {
		return fiber.ErrForbidden
	}
	c.Locals(operatorKey, status.Operator)
	fiberlog.SetActor(c, status.Operator.Login)
	return c.Next()
}

type localKey string

// operatorKey holds the operator requireAdmin resolved. The key has its own
// type so nothing else in the request can collide with it.
const operatorKey localKey = "sprinter.operator"

func operatorFrom(c fiber.Ctx) *auth.Operator {
	operator, _ := c.Locals(operatorKey).(*auth.Operator)
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

func mutating(method string) bool {
	switch method {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return false
	default:
		return true
	}
}

// bind decodes a JSON body. Fiber's own binder answers a malformed body with a
// 500, which reads in the log as Sprinter's fault rather than the caller's.
func bind(c fiber.Ctx, destination any) error {
	if err := json.Unmarshal(c.Body(), destination); err != nil {
		return badRequest("body must be a JSON object")
	}
	return nil
}

func badRequest(message string) error {
	return fiber.NewError(fiber.StatusBadRequest, message)
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
		status, message = fiber.StatusConflict, "the requested change conflicts with existing data"
	}
	if status >= 500 {
		log.Printf("request failed: %v", err)
	}
	return c.Status(status).JSON(fiber.Map{"error": message})
}

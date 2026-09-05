package auth

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/fiberlog"
)

// decisionLocal is where Require stashes the decision it made, so a handler
// behind the gate can read it without asking OAuthManager a second time.
const decisionLocal = "exo.auth.decision"

// Require builds middleware that refuses the request unless the caller's
// session carries permission on Exo.
//
// Wire it onto the group, not onto individual handlers, so that adding a route
// to an already-gated group cannot forget the check:
//
//	live := app.Group("/v1/live", authz.Require(auth.PermissionLive))
//	cmd  := app.Group("/v1/commands", authz.Require(auth.PermissionCommands))
//	cmd.Post("/stop", h.Stop)
//
// Status mapping:
//
//	no cookie, or OAuthManager says 401  -> 401  (sign in)
//	403, or allowed:false                -> 403  (signed in, not permitted)
//	transport error, unexpected status,
//	  or OAUTH_MANAGER_URL unset         -> 503  (fail closed, never open)
func (c *Client) Require(permission string) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		decision, err := c.Check(ctx.Context(), ctx.Get(fiber.HeaderCookie), permission)
		if err != nil {
			// An authorization service Exo cannot reach is an outage, not a
			// pass. Log it — this is the one branch an operator needs to see.
			if errors.Is(err, ErrNotConfigured) {
				log.Printf("authorization unavailable: %v", err)
			} else {
				log.Printf("authorization check for %q failed: %v", permission, err)
			}
			c.Events.LogAsync(logclient.Error, "Authorization service unavailable", map[string]any{
				"permission": permission,
				"error":      err.Error(),
			})
			fiberlog.SetError(ctx, err)
			return ctx.Status(fiber.StatusServiceUnavailable).
				JSON(fiber.Map{"error": "authorization service unavailable"})
		}
		if !decision.Authenticated {
			return ctx.Status(fiber.StatusUnauthorized).
				JSON(fiber.Map{"error": "authentication required"})
		}
		if !decision.Allowed {
			return ctx.Status(fiber.StatusForbidden).
				JSON(fiber.Map{"error": "permission required", "permission": permission})
		}
		ctx.Locals(decisionLocal, decision)
		return ctx.Next()
	}
}

// DecisionFrom returns the decision Require recorded for this request.
func DecisionFrom(ctx fiber.Ctx) (Decision, bool) {
	decision, ok := ctx.Locals(decisionLocal).(Decision)
	return decision, ok
}

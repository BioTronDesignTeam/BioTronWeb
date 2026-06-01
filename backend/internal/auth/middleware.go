package auth

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

const operatorLocal = "operator"

// RequireSession rejects requests without a valid session cookie and, on
// success, stashes the operator in c.Locals for downstream handlers.
func (h *Handler) RequireSession(c *fiber.Ctx) error {
	token := c.Cookies(SessionCookie)
	if token == "" {
		return fiber.ErrUnauthorized
	}
	op, err := h.Store.GetSessionOperator(c.UserContext(), hashToken(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fiber.ErrUnauthorized
		}
		// A DB blip is not the same as "not logged in" — don't silently log the
		// operator out; surface it and let the client retry.
		log.Printf("auth: session lookup: %v", err)
		return fiber.ErrServiceUnavailable
	}
	c.Locals(operatorLocal, op)
	return c.Next()
}

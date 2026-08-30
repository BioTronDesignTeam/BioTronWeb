package auth

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

const OperatorLocal = "operator"

// RequireSession accepts any unexpired session. Org membership is not
// re-checked here — see DEFERRED.md (C2) before shipping a control path.
func (h *Handler) RequireSession(c *fiber.Ctx) error {
	token := c.Cookies(SessionCookie)
	if token == "" {
		return fiber.ErrUnauthorized
	}
	var op *store.SessionOperator
	var err error
	if h.Cache != nil {
		op, err = h.Cache.GetSessionOperator(c.UserContext(), hashToken(token))
	} else {
		op, err = h.Store.GetSessionOperator(c.UserContext(), hashToken(token))
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrBanned) {
			return fiber.ErrUnauthorized
		}
		log.Printf("auth: session lookup: %v", err)
		return fiber.ErrServiceUnavailable
	}
	c.Locals(OperatorLocal, op)
	return c.Next()
}

func (h *Handler) RequireXHR(c *fiber.Ctx) error {
	if c.Get("X-Requested-With") == "" {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func (h *Handler) RequireSuperuser(c *fiber.Ctx) error {
	op := OperatorFrom(c)
	if op == nil || !op.IsSuperuser {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func (h *Handler) RequireStaff(c *fiber.Ctx) error {
	op := OperatorFrom(c)
	if op == nil || !op.IsStaff() {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func OperatorFrom(c *fiber.Ctx) *store.SessionOperator {
	op, _ := c.Locals(OperatorLocal).(*store.SessionOperator)
	return op
}

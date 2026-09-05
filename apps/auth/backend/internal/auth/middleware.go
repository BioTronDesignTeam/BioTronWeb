package auth

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/fiberlog"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

const OperatorLocal = "operator"

// RequireSession accepts any unexpired session. Org membership is not
// re-checked here — see DEFERRED.md (C2) before shipping a control path.
func (h *Handler) RequireSession(c fiber.Ctx) error {
	token := c.Cookies(SessionCookie)
	if token == "" {
		return fiber.ErrUnauthorized
	}
	var op *store.SessionOperator
	var err error
	if h.Cache != nil {
		op, err = h.Cache.GetSessionOperator(c.Context(), hashToken(token))
	} else {
		op, err = h.Store.GetSessionOperator(c.Context(), hashToken(token))
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrBanned) {
			return fiber.ErrUnauthorized
		}
		fail(c, err, "auth: session lookup")
		return fiber.ErrServiceUnavailable
	}
	c.Locals(OperatorLocal, op)
	// Every route that reaches a handler passes here first, so naming the
	// operator once is what puts an actor on each request event.
	fiberlog.SetActor(c, op.Login)
	return c.Next()
}

func (h *Handler) RequireXHR(c fiber.Ctx) error {
	if c.Get("X-Requested-With") == "" {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func (h *Handler) RequireSuperuser(c fiber.Ctx) error {
	op := OperatorFrom(c)
	if op == nil || !op.IsSuperuser {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func (h *Handler) RequireStaff(c fiber.Ctx) error {
	op := OperatorFrom(c)
	if op == nil || !op.IsStaff() {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func OperatorFrom(c fiber.Ctx) *store.SessionOperator {
	op, _ := c.Locals(OperatorLocal).(*store.SessionOperator)
	return op
}

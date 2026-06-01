package auth

import "github.com/gofiber/fiber/v2"

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
		return fiber.ErrUnauthorized
	}
	c.Locals(operatorLocal, op)
	return c.Next()
}

package auth

import (
	"crypto/subtle"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

const stateCookie = "exo_oauth_state"

// Config holds the subset of runtime config the auth handlers need.
type Config struct {
	FrontendURL    string
	CookieSecure   bool
	CookieSameSite string
	SessionTTL     time.Duration
	AdminToken     string
}

type Handler struct {
	Store  *store.Store
	GitHub *GitHubClient
	Cfg    Config
}

// LoginRedirect starts the OAuth flow: it stores a random state in a cookie
// (CSRF defense) and redirects to GitHub's authorize page.
func (h *Handler) LoginRedirect(c *fiber.Ctx) error {
	state, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	// Pinned to Lax, not the session's configurable SameSite: the GitHub->callback
	// hop is a cross-site top-level GET that a Strict cookie would not be sent on.
	c.Cookie(&fiber.Cookie{
		Name:     stateCookie,
		Value:    state,
		Path:     "/",
		HTTPOnly: true,
		Secure:   h.Cfg.CookieSecure,
		SameSite: fiber.CookieSameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})
	return c.Redirect(h.GitHub.AuthCodeURL(state), fiber.StatusFound)
}

// Callback completes the OAuth flow: verify state, exchange the code, confirm
// org membership, persist the operator + session, and set the session cookie.
func (h *Handler) Callback(c *fiber.Ctx) error {
	state := c.Query("state")
	code := c.Query("code")
	cookieState := c.Cookies(stateCookie)

	// The state cookie is single-use; clear it regardless of the outcome.
	clearStateCookie(c, h.Cfg)

	if state == "" || cookieState == "" ||
		subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return c.Status(fiber.StatusBadRequest).SendString("invalid OAuth state")
	}
	if code == "" {
		return c.Status(fiber.StatusBadRequest).SendString("missing code")
	}

	ctx := c.UserContext()
	tok, err := h.GitHub.Exchange(ctx, code)
	if err != nil {
		log.Printf("auth: oauth exchange: %v", err)
		return c.Status(fiber.StatusBadGateway).SendString("OAuth exchange failed")
	}

	member, err := h.GitHub.IsOrgMember(ctx, tok)
	if err != nil {
		log.Printf("auth: org membership check: %v", err)
		return c.Status(fiber.StatusBadGateway).SendString("org membership check failed")
	}
	if !member {
		return c.Redirect(h.Cfg.FrontendURL+"/?auth=denied", fiber.StatusFound)
	}

	user, err := h.GitHub.FetchUser(ctx, tok)
	if err != nil {
		log.Printf("auth: fetch user: %v", err)
		return c.Status(fiber.StatusBadGateway).SendString("failed to fetch user")
	}

	if err := h.Store.UpsertOperator(ctx, store.Operator{
		GitHubID:  user.ID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}); err != nil {
		log.Printf("auth: upsert operator: %v", err)
		return fiber.ErrInternalServerError
	}

	token, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if err := h.Store.CreateSession(ctx, hashToken(token), user.ID,
		time.Now().Add(h.Cfg.SessionTTL), c.Get("User-Agent")); err != nil {
		log.Printf("auth: create session: %v", err)
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.SessionTTL)
	return c.Redirect(h.Cfg.FrontendURL, fiber.StatusFound)
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	if token := c.Cookies(SessionCookie); token != "" {
		if err := h.Store.DeleteSession(c.UserContext(), hashToken(token)); err != nil {
			log.Printf("auth: delete session on logout: %v", err)
		}
	}
	clearSessionCookie(c, h.Cfg.CookieSecure, h.Cfg.CookieSameSite)
	return c.SendStatus(fiber.StatusNoContent)
}

// Me returns the operator behind the current session (set by RequireSession).
func (h *Handler) Me(c *fiber.Ctx) error {
	op, ok := c.Locals(operatorLocal).(*store.SessionOperator)
	if !ok || op == nil {
		return fiber.ErrUnauthorized
	}
	return c.JSON(fiber.Map{
		"github_id":  op.GitHubID,
		"login":      op.Login,
		"name":       op.Name,
		"avatar_url": op.AvatarURL,
	})
}

func clearStateCookie(c *fiber.Ctx, cfg Config) {
	c.Cookie(&fiber.Cookie{
		Name:     stateCookie,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: fiber.CookieSameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour),
	})
}

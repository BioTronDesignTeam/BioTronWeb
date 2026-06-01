package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

// Guests share a single sentinel operator row (real GitHub ids are >= 1).
const (
	guestGitHubID int64 = 0
	guestLogin          = "guest"
)

// newGuestKey returns a 12-char base32 code (~60 bits of entropy).
func newGuestKey() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return s[:12]
}

// formatGuestKey groups the canonical code as XXXX-XXXX-XXXX for display.
func formatGuestKey(k string) string {
	var sb strings.Builder
	for i, r := range k {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// normalizeGuestKey strips formatting so users may type with or without dashes.
func normalizeGuestKey(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// RequireAdmin gates admin routes behind the ADMIN_TOKEN header. If the token is
// unset the route is disabled (404).
func (h *Handler) RequireAdmin(c *fiber.Ctx) error {
	if h.Cfg.AdminToken == "" {
		return fiber.ErrNotFound
	}
	if subtle.ConstantTimeCompare([]byte(c.Get("X-Admin-Token")), []byte(h.Cfg.AdminToken)) != 1 {
		return fiber.ErrUnauthorized
	}
	return c.Next()
}

// AdminGuestKey returns today's guest key (creating it on first request).
func (h *Handler) AdminGuestKey(c *fiber.Ctx) error {
	gk, err := h.Store.EnsureGuestKey(c.UserContext(), newGuestKey())
	if err != nil {
		log.Printf("auth: ensure guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	return c.JSON(fiber.Map{"day": gk.Day, "key": formatGuestKey(gk.Key)})
}

// GuestLogin validates today's guest key and starts a shared guest session.
func (h *Handler) GuestLogin(c *fiber.Ctx) error {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Key) == "" {
		return c.Status(fiber.StatusBadRequest).SendString("missing key")
	}

	gk, err := h.Store.EnsureGuestKey(c.UserContext(), newGuestKey())
	if err != nil {
		log.Printf("auth: ensure guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	if subtle.ConstantTimeCompare([]byte(normalizeGuestKey(body.Key)), []byte(gk.Key)) != 1 {
		return c.Status(fiber.StatusUnauthorized).SendString("invalid or expired key")
	}

	if err := h.Store.UpsertOperator(c.UserContext(), store.Operator{
		GitHubID: guestGitHubID,
		Login:    guestLogin,
		Name:     "Guest",
	}); err != nil {
		log.Printf("auth: upsert guest operator: %v", err)
		return fiber.ErrInternalServerError
	}

	token, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if err := h.Store.CreateSession(c.UserContext(), hashToken(token), guestGitHubID,
		time.Now().Add(h.Cfg.SessionTTL), c.Get("User-Agent")); err != nil {
		log.Printf("auth: create guest session: %v", err)
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.SessionTTL)
	return c.SendStatus(fiber.StatusNoContent)
}

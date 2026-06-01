package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"log"
	"strings"
	"time"
	_ "time/tzdata" // embed zoneinfo so America/Toronto loads without OS tzdata

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

// Guests share a single sentinel operator row (real GitHub ids are >= 1).
const (
	guestGitHubID int64 = 0
	guestLogin          = "guest"
)

// easternZone is Waterloo/Toronto local time (EDT/EST, DST-aware), matching
// Postgres `AT TIME ZONE 'America/Toronto'`. Falls back to fixed EST only if
// tzdata is unavailable. See the time-zone convention in CLAUDE.md.
var easternZone = func() *time.Location {
	if loc, err := time.LoadLocation("America/Toronto"); err == nil {
		return loc
	}
	return time.FixedZone("EST", -5*60*60)
}()

// guestSessionExpiry returns the next Eastern midnight — the instant the daily
// guest key rotates — so a guest session only lasts until the key it used expires.
func guestSessionExpiry(now time.Time) time.Time {
	t := now.In(easternZone)
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, easternZone)
}

// newGuestKey returns a 12-char base32 code (~60 bits of entropy).
func newGuestKey() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return s[:12], nil
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
	candidate, err := newGuestKey()
	if err != nil {
		log.Printf("auth: generate guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	gk, err := h.Store.EnsureGuestKey(c.UserContext(), candidate)
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

	candidate, err := newGuestKey()
	if err != nil {
		log.Printf("auth: generate guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	gk, err := h.Store.EnsureGuestKey(c.UserContext(), candidate)
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
	// Guest sessions expire when the key rotates (next EST midnight), not after
	// the operator SessionTTL.
	expiry := guestSessionExpiry(time.Now())
	if err := h.Store.CreateSession(c.UserContext(), hashToken(token), guestGitHubID,
		expiry, c.Get("User-Agent")); err != nil {
		log.Printf("auth: create guest session: %v", err)
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, time.Until(expiry))
	return c.SendStatus(fiber.StatusNoContent)
}

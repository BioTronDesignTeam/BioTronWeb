package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"log"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/gofiber/fiber/v2"

	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

// easternZone is Waterloo/Toronto local time (EDT/EST, DST-aware).
var easternZone = func() *time.Location {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		panic("auth: load America/Toronto (is time/tzdata embedded?): " + err.Error())
	}
	return loc
}()

// guestSessionExpiry returns the next Eastern midnight — the instant the daily
// guest key rotates — so a guest session only lasts until the key it used expires.
func guestSessionExpiry(now time.Time) time.Time {
	t := now.In(easternZone)
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, easternZone)
}

func easternDay(now time.Time) string {
	return now.In(easternZone).Format("2006-01-02")
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

func normalizeGuestKey(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// StaffGuestKey returns today's guest key (creating it on first request).
func (h *Handler) StaffGuestKey(c *fiber.Ctx) error {
	candidate, err := newGuestKey()
	if err != nil {
		log.Printf("auth: generate guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	gk, err := h.Store.EnsureGuestKey(c.UserContext(), easternDay(time.Now()), candidate)
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing key"})
	}

	now := time.Now()
	candidate, err := newGuestKey()
	if err != nil {
		log.Printf("auth: generate guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	gk, err := h.Store.EnsureGuestKey(c.UserContext(), easternDay(now), candidate)
	if err != nil {
		log.Printf("auth: ensure guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	if subtle.ConstantTimeCompare([]byte(normalizeGuestKey(body.Key)), []byte(gk.Key)) != 1 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired key"})
	}

	if err := h.Store.UpsertOperator(c.UserContext(), store.Operator{
		GitHubID: store.GuestGitHubID,
		Login:    store.GuestLogin,
		Name:     "Guest",
	}, false); err != nil {
		log.Printf("auth: upsert guest operator: %v", err)
		return fiber.ErrInternalServerError
	}

	token, err := newToken()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	expiry := guestSessionExpiry(now)
	if err := h.Store.CreateSession(c.UserContext(), hashToken(token), store.GuestGitHubID,
		expiry, c.Get("User-Agent")); err != nil {
		log.Printf("auth: create guest session: %v", err)
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, time.Until(expiry))
	return c.SendStatus(fiber.StatusNoContent)
}

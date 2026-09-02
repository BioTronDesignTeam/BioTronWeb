package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
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

// StaffProductDailyKeys returns today's independently generated key for every
// product that has opted into daily-key guest access.
func (h *Handler) StaffProductDailyKeys(c *fiber.Ctx) error {
	apps, err := h.Store.ListDailyKeyApps(c.UserContext())
	if err != nil {
		log.Printf("auth: list daily-key products: %v", err)
		return fiber.ErrInternalServerError
	}
	keys := make([]store.ProductDailyKey, 0, len(apps))
	day := easternDay(time.Now())
	for _, app := range apps {
		candidate, err := newGuestKey()
		if err != nil {
			log.Printf("auth: generate product daily key: %v", err)
			return fiber.ErrInternalServerError
		}
		key, err := h.Store.EnsureProductDailyKey(c.UserContext(), app.ID, day, candidate)
		if err != nil {
			log.Printf("auth: ensure product daily key for %s: %v", app.ID, err)
			return fiber.ErrInternalServerError
		}
		key.AppName = app.Name
		key.Key = formatGuestKey(key.Key)
		keys = append(keys, key)
	}
	return c.JSON(keys)
}

// GuestLogin validates today's key for one product and starts a session scoped
// to that product. A key from one app can never authenticate another app.
func (h *Handler) GuestLogin(c *fiber.Ctx) error {
	var body struct {
		AppID string `json:"app_id"`
		Key   string `json:"key"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.AppID) == "" || strings.TrimSpace(body.Key) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "app_id and key required"})
	}

	now := time.Now()
	candidate, err := newGuestKey()
	if err != nil {
		log.Printf("auth: generate guest key: %v", err)
		return fiber.ErrInternalServerError
	}
	gk, err := h.Store.EnsureProductDailyKey(c.UserContext(), strings.TrimSpace(body.AppID), easternDay(now), candidate)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired key"})
		}
		log.Printf("auth: ensure product daily key: %v", err)
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
	if err := h.Store.CreateSession(c.UserContext(), hashToken(token), store.GuestGitHubID, gk.AppID,
		expiry, c.Get("User-Agent")); err != nil {
		log.Printf("auth: create guest session: %v", err)
		return fiber.ErrInternalServerError
	}

	setSessionCookie(c, token, h.Cfg.CookieSecure, h.Cfg.CookieSameSite, h.Cfg.CookieDomain, time.Until(expiry))
	return c.SendStatus(fiber.StatusNoContent)
}

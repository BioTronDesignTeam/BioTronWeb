package auth

import (
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

const returnCookie = "oauth_return"

func (h *Handler) safeReturn(raw string) string {
	fallback := strings.TrimRight(h.Cfg.FrontendURL, "/")
	if raw == "" {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fallback
	}
	origin := u.Scheme + "://" + u.Host
	for _, allowed := range h.Cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(allowed, "/"), origin) {
			return origin
		}
	}
	return fallback
}

func setReturnCookie(c fiber.Ctx, dest string, cfg Config) {
	c.Cookie(&fiber.Cookie{
		Name:     returnCookie,
		Value:    dest,
		Path:     "/",
		HTTPOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: fiber.CookieSameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})
}

func clearReturnCookie(c fiber.Ctx, cfg Config) {
	c.Cookie(&fiber.Cookie{
		Name:     returnCookie,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: fiber.CookieSameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour),
	})
}

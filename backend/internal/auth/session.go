package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const SessionCookie = "oauth_session"

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sameSite(s string) string {
	switch strings.ToLower(s) {
	case "none":
		return fiber.CookieSameSiteNoneMode
	case "strict":
		return fiber.CookieSameSiteStrictMode
	default:
		return fiber.CookieSameSiteLaxMode
	}
}

func setSessionCookie(c *fiber.Ctx, token string, secure bool, sameSiteStr string, ttl time.Duration) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		Secure:   secure,
		SameSite: sameSite(sameSiteStr),
		Expires:  time.Now().Add(ttl),
	})
}

func clearSessionCookie(c *fiber.Ctx, secure bool, sameSiteStr string) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   secure,
		SameSite: sameSite(sameSiteStr),
		Expires:  time.Now().Add(-time.Hour),
	})
}

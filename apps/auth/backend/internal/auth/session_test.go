package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestSessionCookieUsesConfiguredDomain(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		setSessionCookie(c, "test-token", true, "Lax", ".biotron.ca", time.Hour)
		return c.SendStatus(fiber.StatusNoContent)
	})

	response, err := app.Test(httptest.NewRequest("GET", "https://auth.biotron.ca/", nil))
	if err != nil {
		t.Fatal(err)
	}
	setCookie := response.Header.Get("Set-Cookie")
	lowerCookie := strings.ToLower(setCookie)
	if !strings.Contains(lowerCookie, "domain=biotron.ca") && !strings.Contains(lowerCookie, "domain=.biotron.ca") {
		t.Fatalf("session cookie is missing configured domain: %q", setCookie)
	}
	if !strings.Contains(lowerCookie, "httponly") || !strings.Contains(lowerCookie, "secure") {
		t.Fatalf("session cookie is missing security attributes: %q", setCookie)
	}
}

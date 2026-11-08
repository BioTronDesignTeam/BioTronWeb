package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionCookieUsesConfiguredDomain(t *testing.T) {
	response := httptest.NewRecorder()
	setSessionCookie(response, "test-token", true, "Lax", ".biotron.ca", time.Hour)

	setCookie := response.Header().Get("Set-Cookie")
	lowerCookie := strings.ToLower(setCookie)
	if !strings.Contains(lowerCookie, "domain=biotron.ca") && !strings.Contains(lowerCookie, "domain=.biotron.ca") {
		t.Fatalf("session cookie is missing configured domain: %q", setCookie)
	}
	for _, attribute := range []string{"httponly", "secure", "samesite=lax", "path=/", "expires="} {
		if !strings.Contains(lowerCookie, attribute) {
			t.Fatalf("session cookie is missing %s: %q", attribute, setCookie)
		}
	}
}

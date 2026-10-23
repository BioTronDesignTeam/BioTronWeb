package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

const SessionCookie = "oauth_session"

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

// cookieValue returns the value of the named request cookie, or "".
func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool, sameSiteStr, domain string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite(sameSiteStr),
		Domain:   domain,
		Expires:  time.Now().Add(ttl),
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool, sameSiteStr, domain string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite(sameSiteStr),
		Domain:   domain,
		Expires:  time.Now().Add(-time.Hour),
	})
}

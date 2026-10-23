package auth

import (
	"net/http"
	"net/url"
	"strings"
	"time"
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

func setReturnCookie(w http.ResponseWriter, dest string, cfg Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     returnCookie,
		Value:    dest,
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})
}

func clearReturnCookie(w http.ResponseWriter, cfg Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     returnCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour),
	})
}

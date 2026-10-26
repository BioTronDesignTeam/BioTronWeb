package auth

import (
	"net/url"
	"strings"
)

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

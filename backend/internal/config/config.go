package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the backend's runtime configuration, sourced from environment
// variables. See ../.env.example for the full list.
type Config struct {
	Port               string
	DatabaseURL        string
	RedisURL           string
	CacheTTL           time.Duration
	GitHubClientID     string
	GitHubClientSecret string
	GitHubOrg          string
	CallbackURL        string
	FrontendURL        string
	AllowedOrigins     []string
	CookieSecure       bool
	CookieSameSite     string
	CookieDomain       string
	SessionTTL         time.Duration
	TrustedProxies     []string
	SuperuserIDs       map[int64]bool
}

func Load() Config {
	frontendURL := getenv("FRONTEND_URL", "http://localhost:5173")
	return Config{
		Port:               getenv("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           getenv("REDIS_URL", "redis://127.0.0.1:6379/0"),
		CacheTTL:           time.Duration(getint("CACHE_TTL_SECONDS", 300)) * time.Second,
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubOrg:          getenv("GITHUB_ORG", "BioTronDesignTeam"),
		CallbackURL:        getenv("OAUTH_CALLBACK_URL", "http://localhost:8080/auth/github/callback"),
		FrontendURL:        frontendURL,
		AllowedOrigins:     parseOrigins(frontendURL, os.Getenv("CORS_ORIGINS")),
		CookieSecure:       getbool("COOKIE_SECURE", false),
		CookieSameSite:     getenv("COOKIE_SAMESITE", "Lax"),
		CookieDomain:       strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),
		SessionTTL:         time.Duration(getint("SESSION_TTL_HOURS", 168)) * time.Hour,
		TrustedProxies:     splitCSV(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		SuperuserIDs:       parseIntSet(os.Getenv("SUPERUSER_GITHUB_IDS")),
	}
}

func (c Config) IsSuperuserID(id int64) bool {
	return c.SuperuserIDs[id]
}

func (c Config) Validate() error {
	switch strings.ToLower(c.CookieSameSite) {
	case "lax", "none", "strict":
	default:
		return fmt.Errorf("COOKIE_SAMESITE must be Lax, None, or Strict (got %q)", c.CookieSameSite)
	}
	if strings.EqualFold(c.CookieSameSite, "none") && !c.CookieSecure {
		return errors.New("COOKIE_SAMESITE=None requires COOKIE_SECURE=true")
	}
	if strings.ContainsAny(c.CookieDomain, "/: \t\r\n") {
		return errors.New("COOKIE_DOMAIN must be a bare domain such as .biotron.ca")
	}
	return nil
}

func parseIntSet(s string) map[int64]bool {
	out := map[int64]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		out[n] = true
	}
	return out
}

func parseOrigins(frontendURL, extra string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}
	add(frontendURL)
	for _, p := range splitCSV(extra) {
		add(p)
	}
	return out
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getint(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the backend's runtime configuration, sourced from environment
// variables. See backend/.env.example for the full list.
type Config struct {
	Port               string
	DatabaseURL        string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubOrg          string
	CallbackURL        string
	FrontendURL        string
	CookieSecure       bool
	CookieSameSite     string
	SessionTTL         time.Duration
	AdminToken         string
}

func Load() Config {
	return Config{
		Port:               getenv("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubOrg:          getenv("GITHUB_ORG", "BioTronDesignTeam"),
		CallbackURL:        getenv("OAUTH_CALLBACK_URL", "http://localhost:8080/auth/github/callback"),
		FrontendURL:        getenv("FRONTEND_URL", "http://localhost:5173"),
		CookieSecure:       getbool("COOKIE_SECURE", false),
		CookieSameSite:     getenv("COOKIE_SAMESITE", "Lax"),
		SessionTTL:         time.Duration(getint("SESSION_TTL_HOURS", 168)) * time.Hour,
		AdminToken:         os.Getenv("ADMIN_TOKEN"),
	}
}

// Validate rejects configurations a browser would silently reject: a
// SameSite=None cookie without Secure is dropped, breaking cross-origin login.
func (c Config) Validate() error {
	if strings.EqualFold(c.CookieSameSite, "none") && !c.CookieSecure {
		return errors.New("COOKIE_SAMESITE=None requires COOKIE_SECURE=true (browsers drop a SameSite=None cookie that isn't Secure)")
	}
	return nil
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

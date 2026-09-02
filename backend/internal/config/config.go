package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port             string
	DatabaseURL      string
	FrontendURL      string
	SiteURL          string
	PublicBaseURL    string
	OAuthManagerURL  string
	CORSOrigins      []string
	AdminCORSOrigins []string
	TrustedProxies   []string
	MaxRangeDays     int
	DefaultTimezone  string
}

func Load() Config {
	return Config{
		Port:             getenv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		FrontendURL:      strings.TrimRight(getenv("FRONTEND_URL", "http://localhost:5176"), "/"),
		SiteURL:          strings.TrimRight(os.Getenv("SITE_URL"), "/"),
		PublicBaseURL:    strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:18084"), "/"),
		OAuthManagerURL:  strings.TrimRight(getenv("OAUTH_MANAGER_URL", "http://oauth-manager:8080"), "/"),
		CORSOrigins:      splitCSV(os.Getenv("CORS_ORIGINS")),
		AdminCORSOrigins: splitCSV(os.Getenv("ADMIN_CORS_ORIGINS")),
		TrustedProxies:   splitCSV(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		MaxRangeDays:     getint("MAX_RANGE_DAYS", 370),
		DefaultTimezone:  getenv("DEFAULT_TIMEZONE", "America/Toronto"),
	}
}

func (c Config) PublicAllowedOrigins() []string {
	return uniqueOrigins(append([]string{c.FrontendURL, c.SiteURL}, c.CORSOrigins...))
}

func (c Config) AdminAllowedOrigins() []string {
	return uniqueOrigins(append([]string{c.FrontendURL}, c.AdminCORSOrigins...))
}

func uniqueOrigins(origins []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, origin := range origins {
		origin = strings.TrimSpace(strings.TrimRight(origin, "/"))
		if origin != "" && !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	return out
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.MaxRangeDays < 1 || c.MaxRangeDays > 730 {
		return errors.New("MAX_RANGE_DAYS must be between 1 and 730")
	}
	if c.DefaultTimezone != "America/Toronto" {
		return errors.New("DEFAULT_TIMEZONE must be America/Toronto")
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getint(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

package config

import (
	"os"
)

// Config is the backend's runtime configuration, sourced from environment
// variables. See ../.env.example for the full list.
type Config struct {
	Port        string
	FrontendURL string

	OAuthManagerURL string
}

func Load() Config {
	return Config{
		Port:        getenv("PORT", "8080"),
		FrontendURL: getenv("FRONTEND_URL", "http://localhost:5174"),
		// The authorization service every gated route consults. Unset or empty
		// means Auth on the instance network; an unreachable Auth fails every
		// gated route closed with 503.
		OAuthManagerURL: getenv("OAUTH_MANAGER_URL", "http://oauth-manager:8080"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

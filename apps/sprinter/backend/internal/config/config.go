// Package config reads Sprinter's environment once, at start.
//
// Two things here are optional on purpose. An empty DISCORD_TOKEN runs the
// admin API with no gateway session, which is how the tests and a local
// frontend session work. An empty GEMINI_API_KEY runs the bot with the echo
// runner, so a missing key costs the answers and nothing else.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port string
	// DatabaseURL owns the sprinter schema and is the only pool that writes.
	DatabaseURL string
	// ReadDatabaseURL is a read-only role the agent's tools query. Empty leaves
	// out the four SQL tools; the bot still answers from the public endpoints.
	ReadDatabaseURL string
	DiscordToken    string
	DiscordGuildID  string
	GeminiAPIKey    string
	GeminiModel     string
	// ThreadMaxTurns caps one thread's conversation. Every turn is replayed to
	// the model, so an old thread costs more per answer than a new one.
	ThreadMaxTurns int
	// AgentTimeout bounds one whole question, tool calls included.
	AgentTimeout time.Duration
	// PollInterval is how often the scheduler reads Calendar.
	PollInterval    time.Duration
	CalendarURL     string
	LoggerURL       string
	OAuthManagerURL string
	FrontendURL     string
	CORSOrigins     []string
	TrustedProxies  []string
}

func Load() Config {
	return Config{
		Port:            getenv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		ReadDatabaseURL: os.Getenv("READ_DATABASE_URL"),
		DiscordToken:    strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		DiscordGuildID:  strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		GeminiAPIKey:    strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiModel:     getenv("GEMINI_MODEL", "gemini-2.5-flash"),
		ThreadMaxTurns:  getint("THREAD_MAX_TURNS", 40),
		AgentTimeout:    getduration("AGENT_TIMEOUT", 2*time.Minute),
		PollInterval:    getduration("POLL_INTERVAL", 5*time.Minute),
		CalendarURL:     trimURL(getenv("CALENDAR_URL", "http://calendar-api:8080")),
		LoggerURL:       trimURL(getenv("LOGGER_URL", "http://logger-api:8080")),
		OAuthManagerURL: trimURL(getenv("OAUTH_MANAGER_URL", "http://oauth-manager:8080")),
		FrontendURL:     trimURL(getenv("FRONTEND_URL", DefaultFrontendURL)),
		CORSOrigins:     splitCSV(os.Getenv("CORS_ORIGINS")),
		TrustedProxies:  splitCSV(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
	}
}

// ModelEnabled reports whether a real model answers. Without a key the bot
// runs the echo runner rather than refusing to start, because the admin API
// and the guards are still worth having.
func (c Config) ModelEnabled() bool {
	return c.GeminiAPIKey != ""
}

// ReadEnabled reports whether the SQL tools have a pool to read through.
func (c Config) ReadEnabled() bool {
	return strings.TrimSpace(c.ReadDatabaseURL) != ""
}

// DiscordEnabled reports whether the bot should open a gateway session.
func (c Config) DiscordEnabled() bool {
	return c.DiscordToken != ""
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	// Commands are registered per guild, not globally, so a token with no
	// guild id would open a session that can never publish a command.
	if c.DiscordEnabled() && c.DiscordGuildID == "" {
		return errors.New("DISCORD_GUILD_ID is required when DISCORD_TOKEN is set")
	}
	return nil
}

// DefaultFrontendURL is the admin UI's local origin.
const DefaultFrontendURL = "http://localhost:5178"

// AllowedOrigins are the origins that may make credentialed admin requests.
// It never returns an empty list: with credentials allowed, an empty list
// makes Fiber's CORS middleware fall back to the wildcard, which it then
// refuses by panicking at start.
func (c Config) AllowedOrigins() []string {
	seen := map[string]bool{}
	var out []string
	for _, origin := range append([]string{c.FrontendURL}, c.CORSOrigins...) {
		origin = trimURL(origin)
		if origin != "" && !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	if len(out) == 0 {
		return []string{DefaultFrontendURL}
	}
	return out
}

func trimURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
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

// getint and getduration fall back rather than fail. A typo in an optional
// tuning must not stop the bot from starting; the default is always sane.
func getint(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func getduration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

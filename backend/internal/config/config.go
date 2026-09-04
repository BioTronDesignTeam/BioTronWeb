package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	RedisURL              string
	IngestToken           string
	OAuthManagerURL       string
	AuthDisabled          bool
	CatalogJSON           string
	TailSize              int
	HealthInterval        time.Duration
	HealthHistoryInterval time.Duration
	HealthTimeout         time.Duration
	StatusCacheTTL        time.Duration
	StatusRateLimit       int
}

func Load() Config {
	return Config{
		Port:                  getenv("PORT", "8080"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		RedisURL:              getenv("REDIS_URL", "redis://127.0.0.1:6379/0"),
		IngestToken:           os.Getenv("LOGGER_INGEST_TOKEN"),
		OAuthManagerURL:       strings.TrimRight(getenv("OAUTH_MANAGER_URL", "http://oauth-manager:8080"), "/"),
		AuthDisabled:          getbool("AUTH_DISABLED", false),
		CatalogJSON:           os.Getenv("LOGGER_CATALOG_JSON"),
		TailSize:              getint("REDIS_TAIL_SIZE", 500),
		HealthInterval:        getduration("HEALTH_INTERVAL", 15*time.Second),
		HealthHistoryInterval: getduration("HEALTH_HISTORY_INTERVAL", 5*time.Minute),
		HealthTimeout:         getduration("HEALTH_TIMEOUT", 3*time.Second),
		StatusCacheTTL:        getduration("STATUS_CACHE_TTL", 30*time.Second),
		StatusRateLimit:       getint("STATUS_RATE_LIMIT", 60),
	}
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return errors.New("REDIS_URL is required")
	}
	if c.IngestToken == "" {
		return errors.New("LOGGER_INGEST_TOKEN is required")
	}
	if !c.AuthDisabled && c.OAuthManagerURL == "" {
		return errors.New("OAUTH_MANAGER_URL is required when authentication is enabled")
	}
	if c.TailSize < 1 || c.HealthInterval <= 0 || c.HealthHistoryInterval <= 0 || c.HealthTimeout <= 0 {
		return errors.New("tail size and health durations must be positive")
	}
	if c.StatusCacheTTL <= 0 || c.StatusRateLimit < 1 {
		return errors.New("status cache TTL and rate limit must be positive")
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getbool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getint(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func getduration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

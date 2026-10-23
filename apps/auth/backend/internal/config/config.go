package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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
	Port                  string
	DatabaseURL           string
	GitHubAppID           string
	GitHubAppClientID     string
	GitHubAppClientSecret string
	GitHubAppPrivateKey   string
	GitHubOrg             string
	CallbackURL           string
	FrontendURL           string
	AllowedOrigins        []string
	CookieSecure          bool
	CookieSameSite        string
	CookieDomain          string
	SessionTTL            time.Duration
	SessionSigningKey     string
	TrustedProxies        []string
	SuperuserIDs          map[int64]bool
}

func Load() Config {
	frontendURL := getenv("FRONTEND_URL", "http://localhost:5173")
	return Config{
		Port:                  getenv("PORT", "8080"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		GitHubAppID:           strings.TrimSpace(os.Getenv("GITHUB_APP_ID")),
		GitHubAppClientID:     strings.TrimSpace(os.Getenv("GITHUB_APP_CLIENT_ID")),
		GitHubAppClientSecret: strings.TrimSpace(os.Getenv("GITHUB_APP_CLIENT_SECRET")),
		GitHubAppPrivateKey:   os.Getenv("GITHUB_APP_PRIVATE_KEY"),
		GitHubOrg:             getenv("GITHUB_ORG", "BioTronDesignTeam"),
		CallbackURL:           getenv("OAUTH_CALLBACK_URL", "http://localhost:8080/auth/github/callback"),
		FrontendURL:           frontendURL,
		AllowedOrigins:        parseOrigins(frontendURL, os.Getenv("CORS_ORIGINS")),
		CookieSecure:          getbool("COOKIE_SECURE", false),
		CookieSameSite:        getenv("COOKIE_SAMESITE", "Lax"),
		CookieDomain:          strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),
		SessionTTL:            time.Duration(getint("SESSION_TTL_HOURS", 168)) * time.Hour,
		SessionSigningKey:     os.Getenv("SESSION_SIGNING_KEY"),
		TrustedProxies:        splitCSV(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		SuperuserIDs:          parseIntSet(os.Getenv("SUPERUSER_GITHUB_IDS")),
	}
}

func (c Config) IsSuperuserID(id int64) bool {
	return c.SuperuserIDs[id]
}

// Validate refuses a configuration the service must not start with: a cookie
// that a deployment would ship without Secure, or a missing or malformed
// GitHub App credential. Auth fails at startup rather than at the first
// sign-in.
func (c Config) Validate() error {
	if err := c.validateCookies(); err != nil {
		return err
	}
	if err := c.validateSession(); err != nil {
		return err
	}
	return c.validateGitHubApp()
}

func (c Config) validateCookies() error {
	switch strings.ToLower(c.CookieSameSite) {
	case "lax", "none", "strict":
	default:
		return fmt.Errorf("COOKIE_SAMESITE must be Lax, None, or Strict (got %q)", c.CookieSameSite)
	}
	if strings.EqualFold(c.CookieSameSite, "none") && !c.CookieSecure {
		return errors.New("COOKIE_SAMESITE=None requires COOKIE_SECURE=true")
	}
	// COOKIE_SECURE defaults to false so that plain-http localhost development
	// works out of the box, but a false value must never survive into a
	// deployment. Rather than trust the operator to remember one more variable,
	// derive the requirement from configuration that is already deployment
	// shaped: an https callback URL, or a cookie domain. Both are empty or
	// http:// on localhost, so local development is unaffected, and neither can
	// be present in production without Secure also being set.
	if isHTTPSURL(c.CallbackURL) && !c.CookieSecure {
		return errors.New("COOKIE_SECURE=true is required when OAUTH_CALLBACK_URL is https")
	}
	if strings.ContainsAny(c.CookieDomain, "/: \t\r\n") {
		return errors.New("COOKIE_DOMAIN must be a bare domain such as .biotron.ca")
	}
	// A domain cookie is offered to every host under the registrable domain, so
	// shipping it without Secure exposes it to any plaintext request to any
	// sibling subdomain.
	if c.CookieDomain != "" && !c.CookieSecure {
		return errors.New("COOKIE_SECURE=true is required when COOKIE_DOMAIN is set")
	}
	return nil
}

func isHTTPSURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "https://")
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

func (c Config) validateGitHubApp() error {
	var missing []string
	for _, v := range []struct{ name, value string }{
		{"GITHUB_APP_ID", c.GitHubAppID},
		{"GITHUB_APP_CLIENT_ID", c.GitHubAppClientID},
		{"GITHUB_APP_CLIENT_SECRET", c.GitHubAppClientSecret},
		{"GITHUB_APP_PRIVATE_KEY", c.GitHubAppPrivateKey},
	} {
		if strings.TrimSpace(v.value) == "" {
			missing = append(missing, v.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s must be set: create the GitHub App as apps/auth/README.md describes", strings.Join(missing, ", "))
	}
	if id, err := strconv.ParseInt(c.GitHubAppID, 10, 64); err != nil || id <= 0 {
		return errors.New("GITHUB_APP_ID must be the App's numeric id, not its client id")
	}
	if _, err := ParsePrivateKey(c.GitHubAppPrivateKey); err != nil {
		return err
	}
	return nil
}

// ParsePrivateKey reads the App's private key: the PEM file GitHub hands out,
// PKCS#1 or PKCS#8. An .env value cannot always hold line breaks, so a key
// written on one line with literal \n sequences is accepted too. The error
// never repeats the value.
func ParsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	text := strings.TrimSpace(strings.ReplaceAll(raw, `\n`, "\n"))
	block, _ := pem.Decode([]byte(text))
	if block == nil {
		return nil, errors.New("GITHUB_APP_PRIVATE_KEY is not a PEM private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("GITHUB_APP_PRIVATE_KEY is not an RSA private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GITHUB_APP_PRIVATE_KEY is not an RSA private key")
	}
	return key, nil
}

// minSigningKey is the shortest SESSION_SIGNING_KEY accepted: 256 bits, the
// size of the HMAC-SHA256 that signs the session cookie.
const minSigningKey = 32

func (c Config) validateSession() error {
	key := c.SessionSigningKey
	if len(key) < minSigningKey {
		return fmt.Errorf("SESSION_SIGNING_KEY must be at least %d characters: generate one with `openssl rand -base64 48`", minSigningKey)
	}
	// A key of one repeated character, or one copied from an example, is
	// long enough and still guessable.
	distinct := map[rune]bool{}
	for _, r := range key {
		distinct[r] = true
	}
	if len(distinct) < 10 || strings.Contains(strings.ToLower(key), "change-me") {
		return errors.New("SESSION_SIGNING_KEY looks like a placeholder: generate one with `openssl rand -base64 48`")
	}
	if c.SessionTTL <= 0 {
		return errors.New("SESSION_TTL_HOURS must be a positive number of hours")
	}
	return nil
}

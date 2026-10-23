package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func TestCookieDomainValidation(t *testing.T) {
	for _, domain := range []string{"", ".biotron.ca", "biotron.ca"} {
		// A cookie domain now implies Secure (see TestCookieSecureRequired), so
		// the shape test has to supply it to isolate the shape rule.
		if err := (Config{CookieSameSite: "Lax", CookieSecure: true, CookieDomain: domain}).validateCookies(); err != nil {
			t.Fatalf("expected %q to be valid: %v", domain, err)
		}
	}
	for _, domain := range []string{"https://biotron.ca", "biotron.ca/path", "biotron.ca:443"} {
		if err := (Config{CookieSameSite: "Lax", CookieSecure: true, CookieDomain: domain}).validateCookies(); err == nil {
			t.Fatalf("expected %q to be rejected", domain)
		}
	}
}

// Local development must keep working untouched: no COOKIE_SECURE, no
// COOKIE_DOMAIN, and the default http://localhost callback.
func TestLocalhostDefaultsValidate(t *testing.T) {
	for _, key := range []string{
		"OAUTH_CALLBACK_URL", "FRONTEND_URL", "CORS_ORIGINS",
		"COOKIE_SECURE", "COOKIE_SAMESITE", "COOKIE_DOMAIN",
	} {
		t.Setenv(key, "")
	}
	setGitHubApp(t)

	cfg := Load()
	if cfg.CookieSecure {
		t.Fatalf("expected COOKIE_SECURE to default to false, got true")
	}
	if cfg.CookieDomain != "" {
		t.Fatalf("expected empty default COOKIE_DOMAIN, got %q", cfg.CookieDomain)
	}
	if !strings.HasPrefix(cfg.CallbackURL, "http://localhost") {
		t.Fatalf("expected a localhost http callback by default, got %q", cfg.CallbackURL)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("localhost defaults must validate, got: %v", err)
	}
}

func TestCookieSecureRequired(t *testing.T) {
	base := func() Config {
		return Config{
			CookieSameSite: "Lax",
			CallbackURL:    "http://localhost:8080/auth/github/callback",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:   "localhost callback without secure is fine",
			mutate: func(*Config) {},
		},
		{
			name:    "https callback without secure is refused",
			mutate:  func(c *Config) { c.CallbackURL = "https://auth.biotron.ca/auth/github/callback" },
			wantErr: "OAUTH_CALLBACK_URL is https",
		},
		{
			name: "https callback with secure is fine",
			mutate: func(c *Config) {
				c.CallbackURL = "https://auth.biotron.ca/auth/github/callback"
				c.CookieSecure = true
			},
		},
		{
			name:    "https callback is matched case-insensitively",
			mutate:  func(c *Config) { c.CallbackURL = "HTTPS://auth.biotron.ca/auth/github/callback" },
			wantErr: "OAUTH_CALLBACK_URL is https",
		},
		{
			name:    "cookie domain without secure is refused",
			mutate:  func(c *Config) { c.CookieDomain = ".biotron.ca" },
			wantErr: "COOKIE_DOMAIN is set",
		},
		{
			name: "cookie domain with secure is fine",
			mutate: func(c *Config) {
				c.CookieDomain = ".biotron.ca"
				c.CookieSecure = true
			},
		},
		{
			name: "the planned deployment shape is refused when secure is forgotten",
			mutate: func(c *Config) {
				c.CallbackURL = "https://auth.biotron.ca/auth/github/callback"
				c.CookieDomain = ".biotron.ca"
				c.CookieSameSite = "Lax"
			},
			wantErr: "COOKIE_SECURE=true is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(&cfg)
			err := cfg.validateCookies()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("expected valid config, got: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected an error mentioning %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("expected an error mentioning %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// testKeyPEM is one RSA key in PKCS#1 PEM, made once for the package.
var testKeyPEM = func() string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}()

func setGitHubApp(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_CLIENT_ID", "Iv1.test")
	t.Setenv("GITHUB_APP_CLIENT_SECRET", "test-secret")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", testKeyPEM)
}

func TestGitHubAppIsRequiredAtStartup(t *testing.T) {
	for _, key := range []string{"GITHUB_APP_ID", "GITHUB_APP_CLIENT_ID", "GITHUB_APP_CLIENT_SECRET", "GITHUB_APP_PRIVATE_KEY"} {
		t.Run(key, func(t *testing.T) {
			setGitHubApp(t)
			t.Setenv(key, "")
			err := Load().Validate()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("err = %v, want it to name %s", err, key)
			}
		})
	}
}

func TestGitHubAppValuesAreChecked(t *testing.T) {
	for _, tc := range []struct {
		name, key, value, wantErr string
	}{
		{"client id given as the app id", "GITHUB_APP_ID", "Iv1.abc", "numeric id"},
		{"key that is not PEM", "GITHUB_APP_PRIVATE_KEY", "not a key", "not a PEM"},
		{"key on one line with escaped newlines", "GITHUB_APP_PRIVATE_KEY", strings.ReplaceAll(testKeyPEM, "\n", `\n`), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGitHubApp(t)
			t.Setenv(tc.key, tc.value)
			err := Load().Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("expected valid config, got: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestParsePrivateKeyAcceptsPKCS8(t *testing.T) {
	key, err := ParsePrivateKey(testKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKey(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))); err != nil {
		t.Fatalf("PKCS#8: %v", err)
	}
}

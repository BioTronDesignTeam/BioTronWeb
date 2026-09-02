package config

import "testing"

func TestCookieDomainValidation(t *testing.T) {
	for _, domain := range []string{"", ".biotron.ca", "biotron.ca"} {
		if err := (Config{CookieSameSite: "Lax", CookieDomain: domain}).Validate(); err != nil {
			t.Fatalf("expected %q to be valid: %v", domain, err)
		}
	}
	for _, domain := range []string{"https://biotron.ca", "biotron.ca/path", "biotron.ca:443"} {
		if err := (Config{CookieSameSite: "Lax", CookieDomain: domain}).Validate(); err == nil {
			t.Fatalf("expected %q to be rejected", domain)
		}
	}
}

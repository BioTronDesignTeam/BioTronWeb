package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestPublicBaseURLSharesTheBrowserAPIAddress(t *testing.T) {
	for _, test := range []struct {
		name        string
		frontendURL string
		apiURL      string
		override    string
		want        string
	}{
		{name: "local defaults", want: "http://localhost:8083"},
		{name: "absolute API", apiURL: "https://calendar.example/api/", want: "https://calendar.example/api"},
		{name: "relative production API", frontendURL: "https://calendar.uwbiotron.dev/", apiURL: "/api/", want: "https://calendar.uwbiotron.dev/api"},
		{name: "relative local API", apiURL: "/api", want: "http://localhost:5176/api"},
		{name: "explicit subscriber address", frontendURL: "https://calendar.example", apiURL: "/api", override: "https://feeds.example/calendar/", want: "https://feeds.example/calendar"},
		{name: "legacy relative override", frontendURL: "https://calendar.example", apiURL: "/ignored", override: "/api", want: "https://calendar.example/api"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("FRONTEND_URL", test.frontendURL)
			t.Setenv("VITE_API_URL", test.apiURL)
			t.Setenv("PUBLIC_BASE_URL", test.override)
			if got := Load().PublicBaseURL; got != test.want {
				t.Fatalf("PublicBaseURL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateRejectsInvalidFeedAddresses(t *testing.T) {
	for _, address := range []string{"/api", "ftp://calendar.example", "https://", "https://calendar.example/api?token=secret", "https://calendar.example/api#feed", "https://user:pass@calendar.example/api"} { // trufflehog:ignore: made-up addresses the validator must reject
		t.Run(address, func(t *testing.T) {
			config := Config{
				DatabaseURL:     "postgresql://localhost/calendar",
				PublicBaseURL:   address,
				MaxRangeDays:    370,
				DefaultTimezone: "America/Toronto",
			}
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "API address") {
				t.Fatalf("Validate() = %v, want an invalid API address error", err)
			}
		})
	}
}

func TestLoadDefaultsSiteURLToTheConfiguredSitePort(t *testing.T) {
	t.Setenv("SITE_URL", "")
	if got := Load().SiteURL; got != "http://localhost:5177" {
		t.Fatalf("SiteURL = %q, want the local site default", got)
	}
	if warnings := Load().Warnings(); len(warnings) != 0 {
		t.Fatalf("a defaulted SITE_URL must not warn: %v", warnings)
	}
	t.Setenv("SITE_URL", "https://biotron.ca/")
	if got := Load().SiteURL; got != "https://biotron.ca" {
		t.Fatalf("SiteURL = %q, want the trimmed override", got)
	}
}

func TestWarningsNameTheSilentCORSFailure(t *testing.T) {
	warnings := Config{}.Warnings()
	if len(warnings) != 2 {
		t.Fatalf("expected a warning for the missing site origin and for having no public origin at all, got %v", warnings)
	}
	if !strings.Contains(warnings[0], "SITE_URL") || !strings.Contains(warnings[0], "CORS") {
		t.Fatalf("the SITE_URL warning must say what breaks: %q", warnings[0])
	}
}

func TestAllowedOriginsSeparatePublicAndAdminAliases(t *testing.T) {
	config := Config{
		FrontendURL:      "http://localhost:5176",
		SiteURL:          "http://localhost:5177/",
		CORSOrigins:      []string{"http://127.0.0.1:5177", "http://localhost:5176/"},
		AdminCORSOrigins: []string{"http://127.0.0.1:5176", "http://localhost:5176/"},
	}
	publicWant := []string{"http://localhost:5176", "http://localhost:5177", "http://127.0.0.1:5177"}
	if got := config.PublicAllowedOrigins(); !reflect.DeepEqual(got, publicWant) {
		t.Fatalf("PublicAllowedOrigins() = %#v, want %#v", got, publicWant)
	}
	adminWant := []string{"http://localhost:5176", "http://127.0.0.1:5176"}
	if got := config.AdminAllowedOrigins(); !reflect.DeepEqual(got, adminWant) {
		t.Fatalf("AdminAllowedOrigins() = %#v, want %#v", got, adminWant)
	}
}

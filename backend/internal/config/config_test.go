package config

import (
	"reflect"
	"strings"
	"testing"
)

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

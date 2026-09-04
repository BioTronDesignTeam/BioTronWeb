package config

import (
	"reflect"
	"testing"
)

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

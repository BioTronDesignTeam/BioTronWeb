package config

import (
	"reflect"
	"testing"
)

// baseEnv sets the minimum a Logger process needs to start.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://localhost/biotron?schema=logger")
	t.Setenv("LOGGER_INGEST_TOKEN", "test-token")
	t.Setenv("AUTH_DISABLED", "")
	t.Setenv("BIOTRON_ENV", "")
}

// AUTH_DISABLED turns off every read authorization check on an
// internet-reachable service, so it must cost a second deliberate declaration
// that this is not a deployment -- while staying usable for local UI work.
func TestAuthDisabledRequiresTheDevelopmentMarker(t *testing.T) {
	t.Run("off starts normally", func(t *testing.T) {
		baseEnv(t)
		cfg := Load()
		if cfg.AuthDisabled {
			t.Fatal("AuthDisabled should default to false")
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("on without the marker refuses to start", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("AUTH_DISABLED", "true")
		if err := Load().Validate(); err == nil {
			t.Fatal("Validate() = nil; AUTH_DISABLED=true must not start without BIOTRON_ENV=development")
		}
	})

	t.Run("a neighbouring environment is not the marker", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("AUTH_DISABLED", "true")
		t.Setenv("BIOTRON_ENV", "production")
		if err := Load().Validate(); err == nil {
			t.Fatal("Validate() = nil; only BIOTRON_ENV=development may accompany AUTH_DISABLED")
		}
	})

	t.Run("on with the marker starts", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("AUTH_DISABLED", "true")
		t.Setenv("BIOTRON_ENV", "development")
		cfg := Load()
		if !cfg.AuthDisabled {
			t.Fatal("AuthDisabled should be true")
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() = %v; local UI work must still be possible", err)
		}
	})
}

func TestLoadTrustedProxiesDefaultsToLoopbackAndSplitsCSV(t *testing.T) {
	baseEnv(t)

	t.Setenv("TRUSTED_PROXIES", "")
	if got, want := Load().TrustedProxies, []string{"127.0.0.1", "::1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default TrustedProxies = %v, want %v", got, want)
	}

	t.Setenv("TRUSTED_PROXIES", "127.0.0.1, ::1 ,172.16.0.0/12,")
	want := []string{"127.0.0.1", "::1", "172.16.0.0/12"}
	if got := Load().TrustedProxies; !reflect.DeepEqual(got, want) {
		t.Fatalf("TrustedProxies = %v, want %v", got, want)
	}
}

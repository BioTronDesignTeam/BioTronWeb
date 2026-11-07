package config

import (
	"reflect"
	"testing"
)

// baseEnv sets the minimum a Status process needs to start.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://localhost/biotron?schema=logger")
}

func TestDatabaseURLIsTheOnlyRequiredSetting(t *testing.T) {
	baseEnv(t)
	if err := Load().Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	t.Setenv("DATABASE_URL", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("Validate() = nil without DATABASE_URL")
	}
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

// A poll slower than the heartbeat would stretch every history gap past the
// three-heartbeat tolerance once it reached it, and all uptime would read
// unknown. Validate refuses it rather than serve a page of grey bars.
func TestConfigRefusesPollSlowerThanTheGapTolerance(t *testing.T) {
	baseEnv(t)
	for _, c := range []struct {
		interval, history string
		ok                bool
	}{
		{"15s", "5m", true},
		{"5m", "5m", true},
		{"6m", "5m", false},
		{"16m", "5m", false},
	} {
		t.Setenv("HEALTH_INTERVAL", c.interval)
		t.Setenv("HEALTH_HISTORY_INTERVAL", c.history)
		if err := Load().Validate(); (err == nil) != c.ok {
			t.Errorf("HEALTH_INTERVAL=%s HEALTH_HISTORY_INTERVAL=%s: Validate() = %v, want ok=%v", c.interval, c.history, err, c.ok)
		}
	}
}

package config

import (
	"reflect"
	"testing"
)

// baseEnv sets the minimum a Logger process needs to start.
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

package config

import (
	"reflect"
	"testing"
)

func TestLoadTrustedProxiesDefaultsToLoopbackAndSplitsCSV(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://localhost/biotron?schema=logger")
	t.Setenv("LOGGER_INGEST_TOKEN", "test-token")

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

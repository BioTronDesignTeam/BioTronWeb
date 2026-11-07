package main

import (
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

// A start that fails still sends its cause to Grafana: run flushes the events
// before it returns, and only then does main exit.
func TestAFailedStartReachesGrafana(t *testing.T) {
	_, events := logclienttest.New(t, "oauth-manager")
	t.Setenv("SESSION_SIGNING_KEY", "too-short")
	t.Setenv("DATABASE_URL", "")
	err := run()
	if err == nil || !strings.Contains(err.Error(), "SESSION_SIGNING_KEY") {
		t.Fatalf("run = %v, want the config error", err)
	}
	// run has returned, so the event must already have been sent.
	timeout := time.After(time.Second)
	for {
		select {
		case event := <-events:
			if event.Service == "oauth-manager" && event.Message == "OAuthManager failed to start" {
				if cause, _ := event.Payload["error"].(string); !strings.Contains(cause, "SESSION_SIGNING_KEY") {
					t.Fatalf("cause = %q", cause)
				}
				return
			}
		case <-timeout:
			t.Fatal("the failed start never reached Grafana")
		}
	}
}

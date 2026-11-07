package main

import (
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

// A startup failure used to leave through log.Fatalf, which skipped the event
// flush: the configuration warnings and the cause of the failure never reached
// Grafana. run must return the error only after its event was delivered.
func TestStartupFailureStillFlushesItsEvents(t *testing.T) {
	_, sent := logclienttest.New(t, "calendar-api")
	t.Setenv("DATABASE_URL", "postgresql://nobody:nothing@127.0.0.1:1/none?connect_timeout=1")
	previous := connectWait
	connectWait = 0
	t.Cleanup(func() { connectWait = previous })

	err := run()
	if err == nil || !strings.Contains(err.Error(), "connect database") {
		t.Fatalf("run() = %v, want a connect database error", err)
	}

	// Close has already flushed by the time run returns, so the event is
	// waiting in the channel rather than still in flight.
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-sent:
			if event.Message == "Database connection failed" {
				if _, ok := event.Payload["error"]; !ok {
					t.Fatalf("the failure event carried no error: %v", event.Payload)
				}
				return
			}
		case <-deadline:
			t.Fatal("run returned before the failure event was flushed")
		}
	}
}

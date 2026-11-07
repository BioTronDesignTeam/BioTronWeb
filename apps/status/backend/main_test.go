package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/logclienttest"
)

// SIGTERM must let a request already in flight finish, then return cleanly.
func TestServeDrainsInFlightRequestsOnShutdown(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "done")
	})
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopping := make(chan struct{})
	served := make(chan error, 1)
	go func() {
		served <- serve(ctx, newHTTPServer(handler), listener, func() { close(stopping) })
	}()

	body := make(chan string, 1)
	go func() {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/", nil)
		if err != nil {
			body <- "error: " + err.Error()
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			body <- "error: " + err.Error()
			return
		}
		defer response.Body.Close()
		b, _ := io.ReadAll(response.Body)
		body <- string(b)
	}()

	<-entered
	cancel()
	<-stopping
	select {
	case err := <-served:
		t.Fatalf("serve returned %v while a request was still in flight", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	if got := <-body; got != "done" {
		t.Fatalf("in-flight response = %q, want done", got)
	}
	if err := <-served; err != nil {
		t.Fatalf("serve = %v, want nil after a clean shutdown", err)
	}
}

// A listener that fails is reported, not mistaken for a shutdown. Before, the
// error was only logged and Status ran on with no HTTP API until SIGTERM.
func TestServeReportsAServeFailure(t *testing.T) {
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- serve(context.Background(), newHTTPServer(http.NotFoundHandler()), listener, func() {
			t.Error("stopping ran without a shutdown")
		})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serve on a closed listener returned nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve kept running after its listener failed")
	}
}

// The whole process, against a real database: when the port is taken, run
// returns an error, so main exits non-zero and Docker restarts the container.
// Before, it logged the error and kept the monitor running with no HTTP API.
func TestLiveRunFailsWhenThePortIsTaken(t *testing.T) {
	databaseURL := os.Getenv("STATUS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STATUS_TEST_DATABASE_URL is not set")
	}
	taken, err := new(net.ListenConfig).Listen(t.Context(), "tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	_, port, err := net.SplitHostPort(taken.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("PORT", port)
	t.Setenv("STATUS_CATALOG_JSON", `[{"id":"infrastructure","name":"Infrastructure","components":[{"id":"postgres","name":"Database","check":"postgres"}]}]`)
	// The stand-in Grafana gateway receives what run's own client exports.
	_, events := logclienttest.New(t, "status-api")

	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "listen") {
			t.Fatalf("run() = %v, want the listen error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run kept going with its port taken")
	}

	timeout := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Message == "Status API failed" {
				if cause, _ := event.Payload["error"].(string); event.Level != logclient.Error || cause == "" {
					t.Fatalf("failure event = %#v, want an error with the cause", event)
				}
				return
			}
		case <-timeout:
			t.Fatal("run returned without flushing the Status API failed event")
		}
	}
}

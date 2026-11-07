package main

import (
	"context"
	"io"
	"net"
	"net/http"
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

// A listener that fails is reported, not mistaken for a shutdown.
func TestServeReportsAServeFailure(t *testing.T) {
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()

	err = serve(context.Background(), newHTTPServer(http.NotFoundHandler()), listener, func() {
		t.Error("stopping ran without a shutdown")
	})
	if err == nil {
		t.Fatal("serve on a closed listener returned nil")
	}
}

// A port that is taken ends run with an error, so the process exits non-zero
// and Docker restarts it, and the failure reaches Grafana: before, the event
// flush ran only after a served shutdown, so this path dropped every event.
func TestRunFailsAndReportsWhenThePortIsTaken(t *testing.T) {
	taken, err := new(net.ListenConfig).Listen(t.Context(), "tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	_, port, err := net.SplitHostPort(taken.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", port)
	// The stand-in Grafana gateway receives what run's own client exports.
	_, events := logclienttest.New(t, "exogui-api")

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
			if event.Message == "Exo API failed" {
				if cause, _ := event.Payload["error"].(string); event.Level != logclient.Error || cause == "" {
					t.Fatalf("failure event = %#v, want an error with the cause", event)
				}
				return
			}
		case <-timeout:
			t.Fatal("run returned without flushing the Exo API failed event")
		}
	}
}

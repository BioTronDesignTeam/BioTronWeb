package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
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

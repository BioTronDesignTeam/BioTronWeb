package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/exogui/backend/internal/auth"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/exogui/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/exogui/backend/internal/server"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

// shutdownTimeout is how long in-flight requests get to finish on SIGTERM.
const shutdownTimeout = 10 * time.Second

func main() {
	// Set once, before any goroutine reads the clock.
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		log.Fatalf("load America/Toronto timezone: %v", err)
	}
	time.Local = loc

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run serves until SIGINT or SIGTERM. It returns instead of exiting, so its
// deferred calls run on every path.
func run() error {
	_ = godotenv.Load("../.env")

	cfg := config.Load()
	events := logclient.NewFromEnv("exogui-api")
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	// Flushed last, on every path, so a listen failure reaches Grafana too.
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		if err := events.Close(closeCtx); err != nil {
			log.Printf("log export flush: %v", err)
		}
	}()
	// OAUTH_MANAGER_URL defaults to Auth on the instance network, so the client
	// always has a URL. An unreachable Auth fails every gated route closed with
	// 503.
	authz := authclient.New(cfg.OAuthManagerURL, auth.AppID, authclient.WithEvents(events))
	handler := server.New(cfg.FrontendURL, events, authz)
	addr := ":" + cfg.Port

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		events.LogAsync(logclient.Error, "Exo API failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("listen: %w", err)
	}
	events.LogAsync(logclient.Info, "Exo API started", map[string]any{"port": cfg.Port})
	log.Printf("backend listening on %s", addr)

	err = serve(ctx, newHTTPServer(handler), listener, func() {
		log.Println("shutting down")
		events.LogAsync(logclient.Info, "Exo API stopping", nil)
	})
	if err != nil {
		events.LogAsync(logclient.Error, "Exo API failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// newHTTPServer bounds how long a client may take to send a request and how
// long an idle connection stays open. There is no write timeout: a live
// telemetry stream must be able to outlast any fixed deadline.
func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// serve runs srv on listener until ctx ends, then calls stopping and gives
// in-flight requests shutdownTimeout to finish. A failure to serve is
// returned; a clean shutdown is not an error.
func serve(ctx context.Context, srv *http.Server, listener net.Listener, stopping func()) error {
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	stopping()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

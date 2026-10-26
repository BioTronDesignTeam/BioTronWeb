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
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run serves until SIGINT or SIGTERM. It returns instead of exiting, so its
// deferred calls run on every path.
func run() error {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		return fmt.Errorf("load America/Toronto timezone: %w", err)
	}
	time.Local = loc

	_ = godotenv.Load("../.env")

	cfg := config.Load()
	events := logclient.NewFromEnv("exogui-api")
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	authz := authclient.New(cfg.OAuthManagerURL, auth.AppID, authclient.WithEvents(events))
	if !authz.Configured() {
		// Not fatal: /health and the frontend still work. Gated routes fail
		// closed with 503 rather than opening up.
		log.Println("warning: OAUTH_MANAGER_URL unset — every permission-gated route will refuse")
		events.LogAsync(logclient.Warning, "Authorization not configured", nil)
	}
	handler := server.New(cfg.FrontendURL, cfg.TrustedProxies, events, authz)
	addr := ":" + cfg.Port

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	events.LogAsync(logclient.Info, "Exo API started", map[string]any{"port": cfg.Port})
	log.Printf("backend listening on %s", addr)

	err = serve(ctx, newHTTPServer(handler), listener, func() {
		log.Println("shutting down")
		events.LogAsync(logclient.Info, "Exo API stopping", nil)
	})
	// After the server has stopped, so the last request events are queued.
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
	if closeErr := events.Close(closeCtx); closeErr != nil {
		log.Printf("log export flush: %v", closeErr)
	}
	cancelClose()
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// newHTTPServer carries the timeouts the Fiber app had. There is no write
// timeout, as there was none before: a live telemetry stream must be able to
// outlast any fixed deadline.
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

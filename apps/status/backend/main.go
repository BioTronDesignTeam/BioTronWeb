package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/api"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/catalog"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/monitor"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/status/backend/internal/store"
)

// shutdownTimeout is how long in-flight requests get to finish on SIGTERM.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run serves until SIGINT or SIGTERM. It returns instead of exiting, so its
// deferred calls, the store's Close and the event flush among them, run on
// every path. A listener that fails ends it with an error, so the process
// exits non-zero and Docker restarts it.
func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	serviceCatalog, err := catalog.Load(cfg.CatalogJSON)
	if err != nil {
		return err
	}

	// Status's own events, the monitor's among them, go to Grafana and PostHog
	// like every other service's.
	events := logclient.NewFromEnv("status-api")
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	// Deferred first, so it runs last, after the stopping event.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := events.Close(ctx); err != nil {
			log.Printf("flush events: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dataStore, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer dataStore.Close()

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", ":"+cfg.Port)
	if err != nil {
		events.LogAsync(logclient.Error, "Status API failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("listen: %w", err)
	}

	events.LogAsync(logclient.Info, "Status API started", map[string]any{"port": cfg.Port})

	// The monitor and the retention sweep stop with the server, whichever way
	// it stops, and are waited for before the store closes under them.
	workCtx, stopWork := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		stopWork()
		workers.Wait()
	}()
	healthMonitor := monitor.New(dataStore, serviceCatalog, events, cfg.HealthInterval, cfg.HealthHistoryInterval, cfg.HealthTimeout)
	workers.Go(func() { healthMonitor.Run(workCtx) })
	workers.Go(func() { monitor.Retain(workCtx, dataStore, api.HistoryRetention, time.Hour) })

	app := api.New(dataStore, serviceCatalog, api.Options{
		HealthInterval:        cfg.HealthInterval,
		HealthHistoryInterval: cfg.HealthHistoryInterval,
		StatusCacheTTL:        cfg.StatusCacheTTL,
		StatusRateLimit:       cfg.StatusRateLimit,
		TrustedProxies:        cfg.TrustedProxies,
		Events:                events,
	})
	log.Printf("Status API listening on :%s", cfg.Port)
	err = serve(ctx, newHTTPServer(app), listener, func() {
		events.LogAsync(logclient.Info, "Status API stopping", nil)
	})
	if err != nil {
		events.LogAsync(logclient.Error, "Status API failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// newHTTPServer bounds a slow or stuck client. The edge proxies every request,
// and the write bound leaves room for a cold ninety-day status query.
func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
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
		log.Printf("Status API shutdown: %v", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

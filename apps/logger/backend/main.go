package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"

	"github.com/BioTronDesignTeam/Logger/backend/internal/api"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/config"
	"github.com/BioTronDesignTeam/Logger/backend/internal/monitor"
	"github.com/BioTronDesignTeam/Logger/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run serves until SIGINT or SIGTERM. It returns instead of exiting, so its
// deferred calls, the store's Close and the event flush among them, run on
// every path.
func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	serviceCatalog, err := catalog.Load(cfg.CatalogJSON)
	if err != nil {
		return err
	}

	// Logger's own events, the monitor's among them, go to Grafana and PostHog
	// like every other service's.
	events := logclient.NewFromEnv("logger-api")
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
	dataStore, err := store.New(ctx, cfg.DatabaseURL, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer dataStore.Close()

	events.LogAsync(logclient.Info, "Logger API started", map[string]any{"port": cfg.Port})

	healthMonitor := monitor.New(dataStore, serviceCatalog, events, cfg.HealthInterval, cfg.HealthHistoryInterval, cfg.HealthTimeout)
	go healthMonitor.Run(ctx)

	app := api.New(dataStore, serviceCatalog, api.Options{
		HealthInterval:        cfg.HealthInterval,
		HealthHistoryInterval: cfg.HealthHistoryInterval,
		StatusCacheTTL:        cfg.StatusCacheTTL,
		StatusRateLimit:       cfg.StatusRateLimit,
		TrustedProxies:        cfg.TrustedProxies,
		Events:                events,
	})
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: app,
		// The edge proxies every request, so these bound only a slow or stuck
		// client. The write bound leaves room for a cold ninety-day status query.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("Logger API listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Logger API stopped: %v", err)
		}
	}()

	<-ctx.Done()
	events.LogAsync(logclient.Info, "Logger API stopping", nil)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Logger API shutdown: %v", err)
	}
	return nil
}

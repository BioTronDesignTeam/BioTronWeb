package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/config"
	api "github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/server"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/calendar/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run starts Calendar and serves until SIGINT or SIGTERM. It returns instead
// of exiting, so its deferred calls run on every path: a startup failure
// still flushes the warnings queued before it and the event that names it.
func run() error {
	_ = godotenv.Load("../.env")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// The event client is built before anything can block, so a warning about
	// configuration that will break a browser still gets out when the
	// database never comes up and the process dies in connectStore.
	events := logclient.NewFromEnv("calendar-api")
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	// Deferred first, so it runs last, after the stopping event or the
	// failure event.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := events.Close(ctx); err != nil {
			log.Printf("structured log close: %v", err)
		}
	}()
	for _, warning := range cfg.Warnings() {
		log.Printf("warning: %s", warning)
		events.LogAsync(logclient.Warning, "Configuration warning", map[string]any{"warning": warning})
	}
	location, err := time.LoadLocation(cfg.DefaultTimezone)
	if err != nil {
		return fmt.Errorf("load timezone: %w", err)
	}
	calendarStore, err := connectStore(cfg.DatabaseURL)
	if err != nil {
		events.LogAsync(logclient.Error, "Database connection failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("connect database: %w", err)
	}
	defer calendarStore.Close()

	authClient := authclient.New(cfg.OAuthManagerURL, api.AppID)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.New(cfg, calendarStore, authClient, events, location),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	events.LogAsync(logclient.Info, "BiotronCalendar started", map[string]any{"port": cfg.Port})

	served := make(chan error, 1)
	go func() {
		log.Printf("calendar API listening on %s", server.Addr)
		served <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-served:
		events.LogAsync(logclient.Error, "Listener failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	events.LogAsync(logclient.Info, "BiotronCalendar stopping", nil)
	return nil
}

// connectWait is how long connectStore keeps retrying a database that is not
// up yet. Compose starts the API beside Postgres, so the first attempts can
// fail. A test shortens it.
var connectWait = 2 * time.Minute

func connectStore(databaseURL string) (*store.Store, error) {
	deadline := time.Now().Add(connectWait)
	for attempt := 1; ; attempt++ {
		calendarStore, err := store.New(context.Background(), databaseURL)
		if err == nil {
			return calendarStore, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		log.Printf("database not ready (attempt %d): %v; retrying in 3s", attempt, err)
		time.Sleep(3 * time.Second)
	}
}

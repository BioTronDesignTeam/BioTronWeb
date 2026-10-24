package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/auth"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/config"
	api "github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/server"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
)

func main() {
	_ = godotenv.Load("../.env")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	// The event client is built before anything can block, so a warning about
	// configuration that will break a browser is queued even when the
	// database never comes up and the process dies in connectStore.
	events := logclient.NewFromEnv("calendar-api")
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	for _, warning := range cfg.Warnings() {
		log.Printf("warning: %s", warning)
		events.LogAsync(logclient.Warning, "Configuration warning", map[string]any{"warning": warning})
	}
	location, err := time.LoadLocation(cfg.DefaultTimezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	calendarStore, err := connectStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer calendarStore.Close()

	authClient := auth.NewClient(cfg.OAuthManagerURL)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.New(cfg, calendarStore, authClient, events, location),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	events.LogAsync(logclient.Info, "BiotronCalendar started", map[string]any{"port": cfg.Port})

	go func() {
		log.Printf("calendar API listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	events.LogAsync(logclient.Info, "BiotronCalendar stopping", nil)
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelClose()
	// The stopping event is the last one, so the background destinations can
	// send what they hold.
	if err := events.Close(closeCtx); err != nil {
		log.Printf("structured log close: %v", err)
	}
}

func connectStore(databaseURL string) (*store.Store, error) {
	deadline := time.Now().Add(2 * time.Minute)
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

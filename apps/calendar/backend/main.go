package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/auth"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/config"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/server"
	"github.com/BioTronDesignTeam/BiotronCalendar/backend/internal/store"
	"github.com/BioTronDesignTeam/biotron/go/logclient"
)

func main() {
	_ = godotenv.Load("../.env")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	for _, warning := range cfg.Warnings() {
		log.Printf("warning: %s", warning)
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

	events := logclient.NewFromEnv("calendar-api")
	if !events.Enabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}
	authClient := auth.NewClient(cfg.OAuthManagerURL)
	app := server.New(cfg, calendarStore, authClient, events, location)
	address := ":" + cfg.Port
	events.LogAsync(logclient.Info, "BiotronCalendar started", map[string]any{"port": cfg.Port})

	go func() {
		log.Printf("calendar API listening on %s", address)
		if err := app.Listen(address, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	shutdownLogCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := events.Log(shutdownLogCtx, logclient.Info, "BiotronCalendar stopping", nil); err != nil {
		log.Printf("structured shutdown log: %v", err)
	}
	cancel()
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
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

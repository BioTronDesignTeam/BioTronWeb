package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/api"
	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/config"
	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
	"github.com/BioTronDesignTeam/Logger/backend/internal/monitor"
	"github.com/BioTronDesignTeam/Logger/backend/internal/store"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	serviceCatalog, err := catalog.Load(cfg.CatalogJSON)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dataStore, err := store.New(ctx, cfg.DatabaseURL, cfg.RedisURL, cfg.TailSize)
	if err != nil {
		log.Fatal(err)
	}
	defer dataStore.Close()
	recordOwnEvent(dataStore, model.LevelInfo, "Logger API started", map[string]any{"port": cfg.Port})

	var authorizer auth.Authorizer = auth.NewOAuthAuthorizer(cfg.OAuthManagerURL)
	if cfg.AuthDisabled {
		log.Print("warning: Logger read authentication is disabled")
		authorizer = auth.AllowAll{}
	}

	healthMonitor := monitor.New(dataStore, serviceCatalog, cfg.HealthInterval, cfg.HealthHistoryInterval, cfg.HealthTimeout)
	go healthMonitor.Run(ctx)

	app := api.New(dataStore, serviceCatalog, authorizer, api.Options{
		IngestToken:           cfg.IngestToken,
		HealthInterval:        cfg.HealthInterval,
		HealthHistoryInterval: cfg.HealthHistoryInterval,
		StatusCacheTTL:        cfg.StatusCacheTTL,
		StatusRateLimit:       cfg.StatusRateLimit,
		TrustedProxies:        cfg.TrustedProxies,
	})
	go func() {
		log.Printf("Logger API listening on :%s", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("Logger API stopped: %v", err)
		}
	}()

	<-ctx.Done()
	recordOwnEvent(dataStore, model.LevelInfo, "Logger API stopping", nil)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Logger API shutdown: %v", err)
	}
}

func recordOwnEvent(dataStore *store.Store, level model.LogLevel, message string, payload any) {
	var raw json.RawMessage
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			log.Printf("marshal Logger event: %v", err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := dataStore.InsertLog(ctx, model.NewLog{
		Service: "logger-api",
		Level:   level,
		Message: message,
		Payload: raw,
	}); err != nil {
		log.Printf("store Logger event: %v", err)
	}
}

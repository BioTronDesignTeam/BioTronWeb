package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/api"
	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/config"
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

	var authorizer auth.Authorizer = auth.NewOAuthAuthorizer(cfg.OAuthManagerURL)
	if cfg.AuthDisabled {
		log.Print("warning: Logger read authentication is disabled")
		authorizer = auth.AllowAll{}
	}

	healthMonitor := monitor.New(dataStore, serviceCatalog, cfg.HealthInterval, cfg.HealthHistoryInterval, cfg.HealthTimeout)
	go healthMonitor.Run(ctx)

	app := api.New(dataStore, serviceCatalog, authorizer, cfg.IngestToken)
	go func() {
		log.Printf("Logger API listening on :%s", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("Logger API stopped: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Logger API shutdown: %v", err)
	}
}

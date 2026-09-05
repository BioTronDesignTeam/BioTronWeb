package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BioTronDesignTeam/biotron/go/logclient"

	"github.com/BioTronDesignTeam/Logger/backend/internal/api"
	"github.com/BioTronDesignTeam/Logger/backend/internal/auth"
	"github.com/BioTronDesignTeam/Logger/backend/internal/catalog"
	"github.com/BioTronDesignTeam/Logger/backend/internal/config"
	"github.com/BioTronDesignTeam/Logger/backend/internal/monitor"
	"github.com/BioTronDesignTeam/Logger/backend/internal/selflog"
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

	// Logger's own events go straight into the store. Sending them through its
	// ingest route would make the request log record the logging.
	events := selflog.New(dataStore)
	events.LogAsync(logclient.Info, "Logger API started", map[string]any{"port": cfg.Port})

	var authorizer auth.Authorizer = auth.NewOAuthAuthorizer(cfg.OAuthManagerURL)
	if cfg.AuthDisabled {
		log.Print("warning: Logger read authentication is disabled")
		// Every log line and every health detail is now world-readable on a
		// service the edge publishes. One startup line on stdout was the only
		// signal; this puts it in the warehouse an operator actually reads.
		events.LogAsync(logclient.Warning, "Read authentication disabled", nil)
		authorizer = auth.AllowAll{}
	}

	healthMonitor := monitor.New(dataStore, serviceCatalog, events, cfg.HealthInterval, cfg.HealthHistoryInterval, cfg.HealthTimeout)
	go healthMonitor.Run(ctx)

	app := api.New(dataStore, serviceCatalog, authorizer, api.Options{
		IngestToken:           cfg.IngestToken,
		HealthInterval:        cfg.HealthInterval,
		HealthHistoryInterval: cfg.HealthHistoryInterval,
		StatusCacheTTL:        cfg.StatusCacheTTL,
		StatusRateLimit:       cfg.StatusRateLimit,
		IngestRateLimit:       cfg.IngestRateLimit,
		TrustedProxies:        cfg.TrustedProxies,
		Events:                events,
	})
	go func() {
		log.Printf("Logger API listening on :%s", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("Logger API stopped: %v", err)
		}
	}()

	<-ctx.Done()
	// Synchronous: the process is leaving, and a background send would leave
	// with it.
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	events.Log(stopCtx, logclient.Info, "Logger API stopping", nil)
	cancelStop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Logger API shutdown: %v", err)
	}
}

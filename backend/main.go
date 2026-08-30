package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/config"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/eventlog"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/server"
)

func main() {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		log.Fatalf("load America/Toronto timezone: %v", err)
	}
	time.Local = loc

	_ = godotenv.Load("../.env")

	cfg := config.Load()
	events := eventlog.NewFromEnv("exo-api")
	if !events.Enabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}
	app := server.New(cfg.FrontendURL, cfg.TrustedProxies, events)
	addr := ":" + cfg.Port
	events.LogAsync(eventlog.Info, "Exo API started", map[string]any{"port": cfg.Port})

	go func() {
		log.Printf("backend listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	shutdownLogCtx, cancelShutdownLog := context.WithTimeout(context.Background(), 2*time.Second)
	if err := events.Log(shutdownLogCtx, eventlog.Info, "Exo API stopping", nil); err != nil {
		log.Printf("structured shutdown log: %v", err)
	}
	cancelShutdownLog()
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

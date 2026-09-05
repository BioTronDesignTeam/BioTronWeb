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

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
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
	authz := auth.NewClient(cfg.OAuthManagerURL)
	if !authz.Configured() {
		// Not fatal: /health and the frontend still work. Gated routes fail
		// closed with 503 rather than opening up.
		log.Println("warning: OAUTH_MANAGER_URL unset — every permission-gated route will refuse")
	}
	app := server.New(cfg.FrontendURL, cfg.TrustedProxies, events, authz)
	addr := ":" + cfg.Port
	events.LogAsync(eventlog.Info, "Exo API started", map[string]any{"port": cfg.Port})

	go func() {
		log.Printf("backend listening on %s", addr)
		// v3 moved DisableStartupMessage out of fiber.Config and onto Listen.
		if err := app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
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

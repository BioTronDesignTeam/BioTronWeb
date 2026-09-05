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

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/auth"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/discord"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/server"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

func main() {
	_ = godotenv.Load("../.env")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	// The event client is built before anything can block, so a warning still
	// reaches Logger when the database never comes up and the process dies in
	// connectStore.
	events := logclient.NewFromEnv("sprinter")
	if !events.Enabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}

	sprinterStore, err := connectStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer sprinterStore.Close()

	var bot *discord.Bot
	if !cfg.DiscordEnabled() {
		events.LogAsync(logclient.Warning, "Discord token unset", nil)
	} else {
		bot, err = discord.New(cfg.DiscordToken, sprinterStore, events, discord.Options{
			GuildID: cfg.DiscordGuildID,
			// Until the model layer lands, every answer is an echo, and the
			// agent_threads row says so rather than naming a model nothing ran.
			Model:  "echo",
			Runner: discord.EchoRunner{},
		})
		if err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "create", "error": err.Error(),
			})
			log.Fatalf("create Discord session: %v", err)
		}
		if err := bot.Open(); err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "open", "error": err.Error(),
			})
			log.Fatalf("open Discord session: %v", err)
		}
		defer bot.Close()
	}

	authClient := auth.NewClient(cfg.OAuthManagerURL)
	app := server.New(cfg, sprinterStore, authClient, events, bot.Connected)
	address := ":" + cfg.Port
	events.LogAsync(logclient.Info, "Sprinter started", map[string]any{
		"port": cfg.Port, "discord": cfg.DiscordEnabled(),
	})

	go func() {
		log.Printf("Sprinter admin API listening on %s", address)
		if err := app.Listen(address, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	shutdownLogCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := events.Log(shutdownLogCtx, logclient.Info, "Sprinter stopping", nil); err != nil {
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
		sprinterStore, err := store.New(context.Background(), databaseURL)
		if err == nil {
			return sprinterStore, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		log.Printf("database not ready (attempt %d): %v; retrying in 3s", attempt, err)
		time.Sleep(3 * time.Second)
	}
}

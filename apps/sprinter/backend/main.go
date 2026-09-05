package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient/fiberlog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	events := logclient.NewFromEnv("sprinter")
	if !events.Enabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}

	discordConnected := false
	var discord *discordgo.Session
	if token := os.Getenv("DISCORD_TOKEN"); token == "" {
		events.LogAsync(logclient.Warning, "Discord token unset", nil)
	} else {
		var err error
		discord, err = discordgo.New("Bot " + token)
		if err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "create",
				"error": err.Error(),
			})
			log.Fatalf("create Discord session: %v", err)
		}

		// Register handlers before Open so the gateway's first Ready cannot
		// arrive before anyone is listening for it.
		discord.AddHandler(func(_ *discordgo.Session, r *discordgo.Ready) {
			events.LogAsync(logclient.Info, "Discord connected", map[string]any{
				"user":   r.User.Username,
				"guilds": len(r.Guilds),
			})
		})
		discord.AddHandler(func(_ *discordgo.Session, _ *discordgo.Disconnect) {
			events.LogAsync(logclient.Warning, "Discord disconnected", nil)
		})
		discord.AddHandler(func(_ *discordgo.Session, _ *discordgo.Resumed) {
			events.LogAsync(logclient.Info, "Discord resumed", nil)
		})

		if err := discord.Open(); err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "open",
				"error": err.Error(),
			})
			log.Fatalf("open Discord session: %v", err)
		}
		discordConnected = true
		defer discord.Close()
	}

	app := fiber.New()
	app.Use(fiberlog.New(events, fiberlog.Options{}))
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"discord_connected": discordConnected,
			"service":           "sprinter",
			"status":            "ok",
		})
	})

	port := getenv("PORT", "8080")
	events.LogAsync(logclient.Info, "Sprinter started", map[string]any{
		"port":    port,
		"discord": discordConnected,
	})

	go func() {
		log.Printf("Sprinter admin API listening on :%s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Printf("admin API stopped: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownLogCtx, cancelShutdownLog := context.WithTimeout(context.Background(), 2*time.Second)
	if err := events.Log(shutdownLogCtx, logclient.Info, "Sprinter stopping", nil); err != nil {
		log.Printf("structured shutdown log: %v", err)
	}
	cancelShutdownLog()
	if err := app.Shutdown(); err != nil {
		log.Printf("shut down admin API: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

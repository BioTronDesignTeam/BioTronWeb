package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/gofiber/fiber/v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	discordConnected := false
	var discord *discordgo.Session
	if token := os.Getenv("DISCORD_TOKEN"); token != "" {
		var err error
		discord, err = discordgo.New("Bot " + token)
		if err != nil {
			log.Fatalf("create Discord session: %v", err)
		}
		if err := discord.Open(); err != nil {
			log.Fatalf("open Discord session: %v", err)
		}
		discordConnected = true
		defer discord.Close()
	}

	app := fiber.New()
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"discord_connected": discordConnected,
			"service":           "sprinter",
			"status":            "ok",
		})
	})

	port := getenv("PORT", "8080")
	go func() {
		log.Printf("Sprinter admin API listening on :%s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Printf("admin API stopped: %v", err)
		}
	}()

	<-ctx.Done()
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

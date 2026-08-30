package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
)

func main() {
	app := fiber.New()

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "biotron-calendar",
			"status":  "ok",
		})
	})

	port := getenv("PORT", "8080")
	log.Printf("calendar API listening on :%s", port)
	log.Fatal(app.Listen(":" + port))
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

package main

import (
	"context"
	"log"
	"time"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/config"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/server"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

func main() {
	// Render all local times (logs, etc.) in Waterloo/Toronto Eastern. See CLAUDE.md.
	if loc, err := time.LoadLocation("America/Toronto"); err == nil {
		time.Local = loc
	}

	// Load backend/.env if present; real environment variables take precedence.
	_ = godotenv.Load()

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		log.Println("warning: GITHUB_CLIENT_ID/SECRET unset — operator login will fail until configured")
	}

	st, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer st.Close()

	gh := auth.NewGitHubClient(auth.GitHubOptions{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		CallbackURL:  cfg.CallbackURL,
		Org:          cfg.GitHubOrg,
	})

	h := &auth.Handler{
		Store:  st,
		GitHub: gh,
		Cfg: auth.Config{
			FrontendURL:    cfg.FrontendURL,
			CookieSecure:   cfg.CookieSecure,
			CookieSameSite: cfg.CookieSameSite,
			SessionTTL:     cfg.SessionTTL,
			AdminToken:     cfg.AdminToken,
		},
	}

	app := server.New(h, cfg.FrontendURL)
	addr := ":" + cfg.Port
	log.Printf("backend listening on %s", addr)
	log.Fatal(app.Listen(addr))
}

// connectDB retries because on a cold boot the Postgres container may not be up
// yet, and crash-looping would trip systemd's start limit and park the backend.
func connectDB(databaseURL string) (*store.Store, error) {
	const dbConnectTimeout = 2 * time.Minute
	deadline := time.Now().Add(dbConnectTimeout)
	for attempt := 1; ; attempt++ {
		st, err := store.New(context.Background(), databaseURL)
		if err == nil {
			return st, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		log.Printf("database not ready (attempt %d): %v; retrying in 3s", attempt, err)
		time.Sleep(3 * time.Second)
	}
}

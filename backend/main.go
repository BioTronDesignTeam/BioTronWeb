package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/config"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/server"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

func main() {
	// Load backend/.env if present; real environment variables take precedence.
	_ = godotenv.Load()

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		log.Println("warning: GITHUB_CLIENT_ID/SECRET unset — operator login will fail until configured")
	}

	st, err := store.New(context.Background(), cfg.DatabaseURL)
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

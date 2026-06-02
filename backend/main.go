package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed zoneinfo so America/Toronto loads without OS tzdata

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/config"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/server"
	"github.com/BioTronDesignTeam/exo-gui/backend/internal/store"
)

func main() {
	// Render all local times (logs, etc.) in Waterloo/Toronto Eastern. tzdata is
	// embedded above, so a failure here means a broken build, not a missing OS zone.
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		log.Fatalf("load America/Toronto timezone: %v", err)
	}
	time.Local = loc

	// Load backend/.env if present; real environment variables take precedence.
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
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

	go prune(st)

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

	app := server.New(h, cfg.FrontendURL, cfg.TrustedProxies)
	addr := ":" + cfg.Port

	go func() {
		log.Printf("backend listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Drain in-flight requests on SIGTERM (the daily 03:00 reboot) so deferred
	// cleanup like st.Close() actually runs instead of being killed mid-request.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// prune sweeps expired session rows and stale guest keys hourly for the life of
// the process.
func prune(st *store.Store) {
	for {
		if n, err := st.DeleteExpiredSessions(context.Background()); err != nil {
			log.Printf("prune sessions: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d expired sessions", n)
		}
		if n, err := st.DeleteOldGuestKeys(context.Background()); err != nil {
			log.Printf("prune guest keys: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d stale guest keys", n)
		}
		time.Sleep(time.Hour)
	}
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

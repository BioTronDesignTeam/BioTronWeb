package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/auth"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/cache"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/config"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/server"
	"github.com/BioTronDesignTeam/oauth-manager/backend/internal/store"
)

func main() {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		log.Fatalf("load America/Toronto timezone: %v", err)
	}
	time.Local = loc

	_ = godotenv.Load("../.env")

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	// Validate has already refused a missing or malformed GitHub App, so the
	// key parses here.
	appKey, err := config.ParsePrivateKey(cfg.GitHubAppPrivateKey)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	events := logclient.NewFromEnv("oauth-manager")
	if !events.LoggerEnabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}

	st, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer st.Close()

	go prune(st, events)

	gh := auth.NewGitHubClient(auth.GitHubOptions{
		AppID:        cfg.GitHubAppID,
		PrivateKey:   appKey,
		ClientID:     cfg.GitHubAppClientID,
		ClientSecret: cfg.GitHubAppClientSecret,
		CallbackURL:  cfg.CallbackURL,
		Org:          cfg.GitHubOrg,
	})

	h := &auth.Handler{
		Store:  st,
		Cache:  cache.New(st),
		GitHub: gh,
		Events: events,
		Cfg: auth.Config{
			FrontendURL:    cfg.FrontendURL,
			AllowedOrigins: cfg.AllowedOrigins,
			CookieSecure:   cfg.CookieSecure,
			CookieSameSite: cfg.CookieSameSite,
			CookieDomain:   cfg.CookieDomain,
			SessionTTL:     cfg.SessionTTL,
			IsSuperuserID:  cfg.IsSuperuserID,
		},
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(h, cfg.AllowedOrigins, cfg.TrustedProxies, events),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	events.LogAsync(logclient.Info, "OAuthManager started", map[string]any{"port": cfg.Port})

	go func() {
		log.Printf("oauth-manager listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	shutdownLogCtx, cancelShutdownLog := context.WithTimeout(context.Background(), 2*time.Second)
	if err := events.Log(shutdownLogCtx, logclient.Info, "OAuthManager stopping", nil); err != nil {
		log.Printf("structured shutdown log: %v", err)
	}
	cancelShutdownLog()
	// In-flight requests get ten seconds to finish; new ones are refused.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	// The last request events are queued now; send what Grafana and PostHog
	// still hold before the process exits.
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFlush()
	if err := events.Close(flushCtx); err != nil {
		log.Printf("log export flush: %v", err)
	}
}

// prune runs every hour and reports only what it changed. A sweep that deletes
// nothing sends nothing, so the log stays a record of events rather than a
// heartbeat.
func prune(st *store.Store, events *logclient.Client) {
	for {
		if n, err := st.DeleteExpiredSessions(context.Background()); err != nil {
			log.Printf("prune sessions: %v", err)
			events.LogAsync(logclient.Error, "Session prune failed", map[string]any{"error": err.Error()})
		} else if n > 0 {
			log.Printf("pruned %d expired sessions", n)
			events.LogAsync(logclient.Info, "Expired sessions pruned", map[string]any{"count": n})
		}
		if n, err := st.DeleteOldProductDailyKeys(context.Background()); err != nil {
			log.Printf("prune product daily keys: %v", err)
			events.LogAsync(logclient.Error, "Guest key prune failed", map[string]any{"error": err.Error()})
		} else if n > 0 {
			log.Printf("pruned %d stale product daily keys", n)
			events.LogAsync(logclient.Info, "Stale guest keys pruned", map[string]any{"count": n})
		}
		time.Sleep(time.Hour)
	}
}

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

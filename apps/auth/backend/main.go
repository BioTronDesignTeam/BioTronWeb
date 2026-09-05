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

	"github.com/BioTronDesignTeam/biotron/go/logclient"
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
	// The client reads only the environment, so it is built before anything can
	// fail. A misconfigured sign-in is then reported to Logger as well as to
	// stdout, instead of being noticed when the first operator cannot get in.
	events := logclient.NewFromEnv("oauth-manager")
	if !events.Enabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		log.Println("warning: GITHUB_CLIENT_ID/SECRET unset — operator login will fail until configured")
		events.LogAsync(logclient.Warning, "GitHub OAuth not configured", nil)
	}

	st, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer st.Close()

	c, err := connectRedis(cfg.RedisURL, st, cfg.CacheTTL)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer c.Close()

	go prune(st, events)

	gh := auth.NewGitHubClient(auth.GitHubOptions{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		CallbackURL:  cfg.CallbackURL,
		Org:          cfg.GitHubOrg,
	})

	h := &auth.Handler{
		Store:  st,
		Cache:  c,
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

	app := server.New(h, cfg.AllowedOrigins, cfg.TrustedProxies, events)
	addr := ":" + cfg.Port
	events.LogAsync(logclient.Info, "OAuthManager started", map[string]any{"port": cfg.Port})

	go func() {
		log.Printf("oauth-manager listening on %s", addr)
		// v3 moved DisableStartupMessage out of fiber.Config and into the
		// per-listener config; the banner stays suppressed.
		if err := app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
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
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
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

func connectRedis(redisURL string, st *store.Store, ttl time.Duration) (*cache.Cache, error) {
	const redisConnectTimeout = 2 * time.Minute
	deadline := time.Now().Add(redisConnectTimeout)
	for attempt := 1; ; attempt++ {
		c, err := cache.New(context.Background(), redisURL, st, ttl)
		if err == nil {
			return c, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		log.Printf("redis not ready (attempt %d): %v; retrying in 3s", attempt, err)
		time.Sleep(3 * time.Second)
	}
}

package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

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
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		log.Println("warning: GITHUB_CLIENT_ID/SECRET unset — operator login will fail until configured")
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

	go prune(st)

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
		Cfg: auth.Config{
			FrontendURL:    cfg.FrontendURL,
			AllowedOrigins: cfg.AllowedOrigins,
			CookieSecure:   cfg.CookieSecure,
			CookieSameSite: cfg.CookieSameSite,
			SessionTTL:     cfg.SessionTTL,
			IsSuperuserID:  cfg.IsSuperuserID,
		},
	}

	app := server.New(h, cfg.AllowedOrigins, cfg.TrustedProxies)
	addr := ":" + cfg.Port

	go func() {
		log.Printf("oauth-manager listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func prune(st *store.Store) {
	for {
		if n, err := st.DeleteExpiredSessions(context.Background()); err != nil {
			log.Printf("prune sessions: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d expired sessions", n)
		}
		if n, err := st.DeleteOldProductDailyKeys(context.Background()); err != nil {
			log.Printf("prune product daily keys: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d stale product daily keys", n)
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

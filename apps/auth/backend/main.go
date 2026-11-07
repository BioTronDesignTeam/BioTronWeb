package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/auth"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/cache"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/server"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/auth/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

func main() {
	// Set before anything runs, since every goroutine reads time.Local. No
	// event client exists yet, so there is nothing to flush.
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		log.Fatalf("load America/Toronto timezone: %v", err)
	}
	time.Local = loc
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run starts Auth and serves until SIGINT or SIGTERM. Every way out, a
// failure to start included, passes through the deferred flush, so the cause
// reaches Grafana before the process exits; main only prints it and exits 1.
func run() error {
	_ = godotenv.Load("../.env")

	events := logclient.NewFromEnv("oauth-manager")
	defer flush(events)
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	startFailed := func(err error) error {
		events.LogAsync(logclient.Error, "OAuthManager failed to start", map[string]any{"error": err.Error()})
		return err
	}

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return startFailed(fmt.Errorf("config: %w", err))
	}
	if cfg.DatabaseURL == "" {
		return startFailed(errors.New("DATABASE_URL is required"))
	}
	// Validate has already refused a missing or malformed GitHub App, so the
	// key parses here.
	appKey, err := config.ParsePrivateKey(cfg.GitHubAppPrivateKey)
	if err != nil {
		return startFailed(fmt.Errorf("config: %w", err))
	}

	st, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		return startFailed(fmt.Errorf("connect db: %w", err))
	}
	defer st.Close()
	// RequireSession reads SUPERUSER_GITHUB_IDS on every request. The column
	// mirrors it for readers of the table, such as Sprinter, so a removed id
	// loses the flag there with this deploy rather than at its next sign-in.
	if n, err := st.ReconcileSuperusers(context.Background(), cfg.SuperuserIDList()); err != nil {
		// Auth itself still reads the list; only the mirror waits for a restart.
		log.Printf("reconcile superusers: %v", err)
	} else if n > 0 {
		log.Printf("superuser flag changed for %d operators from SUPERUSER_GITHUB_IDS", n)
	}

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
		Store:   st,
		Cache:   cache.New(st),
		GitHub:  gh,
		Members: auth.NewMembership(gh.IsOrgMemberByID),
		Signer:  auth.NewSigner([]byte(cfg.SessionSigningKey)),
		Events:  events,
		Cfg: auth.Config{
			FrontendURL:     cfg.FrontendURL,
			AllowedOrigins:  cfg.AllowedOrigins,
			CookieSecure:    cfg.CookieSecure,
			CookieSameSite:  cfg.CookieSameSite,
			CookieDomain:    cfg.CookieDomain,
			SessionTTL:      cfg.SessionTTL,
			IsSuperuserID:   cfg.IsSuperuserID,
			StateCookiePath: auth.StateCookiePath(cfg.CallbackURL),
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

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("oauth-manager listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		events.LogAsync(logclient.Error, "OAuthManager stopped serving", map[string]any{"error": err.Error()})
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	log.Println("shutting down")
	events.LogAsync(logclient.Info, "OAuthManager stopping", nil)
	// In-flight requests get ten seconds to finish; new ones are refused.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	return nil
}

// flush sends what Grafana and PostHog still hold before the process exits:
// the last request events at a shutdown, or the cause of a failed start.
func flush(events *logclient.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := events.Close(ctx); err != nil {
		log.Printf("log export flush: %v", err)
	}
}

// prune runs every hour and reports only what it changed. A sweep that deletes
// nothing sends nothing, so the log stays a record of events rather than a
// heartbeat.
func prune(st *store.Store, events *logclient.Client) {
	for {
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

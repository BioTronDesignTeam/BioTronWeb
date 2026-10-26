package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/agent"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/discord"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/model/gemini"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/readstore"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/scheduler"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/server"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/store"
	"github.com/BioTronDesignTeam/BioTronWeb/apps/sprinter/backend/internal/tools"
	"github.com/BioTronDesignTeam/BioTronWeb/go/authclient"
	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run starts Sprinter and serves until SIGINT or SIGTERM. It returns instead
// of exiting, so its deferred calls run on every path: a startup failure
// still flushes the event that names it.
func run() error {
	_ = godotenv.Load("../.env")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// The event client is built before anything can block, so a warning still
	// gets out when the database never comes up and the process dies in
	// connectStore.
	events := logclient.NewFromEnv("sprinter")
	if !events.Enabled() {
		log.Println("warning: neither Grafana nor PostHog is configured — structured events go nowhere")
	}
	// Deferred first, so it runs last: after the stopping event and whatever
	// the Discord session reports as it closes.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := events.Close(ctx); err != nil {
			log.Printf("flush events: %v", err)
		}
	}()

	sprinterStore, err := connectStore(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer sprinterStore.Close()

	// The read pool is optional. Without it the four SQL tools are left out and
	// the bot answers from the two public endpoints, which is better than
	// refusing to start over a tool nobody may have asked for yet.
	var reader *readstore.Store
	if cfg.ReadEnabled() {
		reader, err = readstore.New(context.Background(), cfg.ReadDatabaseURL)
		if err != nil {
			events.LogAsync(logclient.Error, "Read pool failed", map[string]any{"error": err.Error()})
			return fmt.Errorf("connect read database: %w", err)
		}
		defer reader.Close()
	} else {
		events.LogAsync(logclient.Warning, "Read database unset", nil)
	}

	modelName, runner, answering, err := buildRunner(cfg, reader, events)
	if err != nil {
		return err
	}

	var bot *discord.Bot
	if !cfg.DiscordEnabled() {
		// Off is a deliberate setting, not a fault: production cannot reach
		// Discord. The admin API still runs, and nothing touches the gateway.
		events.LogAsync(logclient.Info, "Discord disabled", nil)
		log.Println("SPRINTER_DISCORD_ENABLED is off — no Discord session, commands, or scheduler")
	} else {
		bot, err = discord.New(cfg.DiscordToken, sprinterStore, events, discord.Options{
			GuildID:        cfg.DiscordGuildID,
			Model:          modelName,
			Runner:         runner,
			MaxThreadTurns: cfg.ThreadMaxTurns,
		})
		if err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "create", "error": err.Error(),
			})
			return fmt.Errorf("create Discord session: %w", err)
		}
		if err := bot.Open(); err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "open", "error": err.Error(),
			})
			return fmt.Errorf("open Discord session: %w", err)
		}
		defer func() {
			if err := bot.Close(); err != nil {
				log.Printf("close Discord session: %v", err)
			}
		}()

		// The scheduler needs a gateway session to post with, so it runs only
		// beside a live bot. Automations are read from Postgres on every tick,
		// which is how an admin change takes effect without a restart.
		location, err := time.LoadLocation("America/Toronto")
		if err != nil {
			return fmt.Errorf("load location: %w", err)
		}
		sched := scheduler.New(scheduler.Deps{
			Store:    sprinterStore,
			Calendar: calendar.NewClient(cfg.CalendarURL, location),
			Discord:  scheduler.NewDiscordSession(bot.Session()),
			Model:    answering,
			Events:   events,
			Location: location,
		})
		schedulerCtx, stopScheduler := context.WithCancel(context.Background())
		defer stopScheduler()
		go sched.Run(schedulerCtx, cfg.PollInterval)
		events.LogAsync(logclient.Info, "Scheduler started", map[string]any{
			"poll_interval": cfg.PollInterval.String(),
		})
	}

	authClient := authclient.New(cfg.OAuthManagerURL, server.AppID)
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: server.New(cfg, sprinterStore, authClient, events, bot.Connected),
		// ReadTimeout covers a slow body as well as slow headers. The
		// WriteTimeout leaves room for the Auth call an admin request makes,
		// bounded at five seconds.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	events.LogAsync(logclient.Info, "Sprinter started", map[string]any{
		"port": cfg.Port, "discord": cfg.DiscordEnabled(),
	})

	served := make(chan error, 1)
	go func() {
		log.Printf("Sprinter admin API listening on %s", httpServer.Addr)
		served <- httpServer.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-served:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	log.Println("shutting down")
	events.LogAsync(logclient.Info, "Sprinter stopping", nil)
	// Requests in flight finish; new connections are refused.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	return nil
}

// buildRunner picks what answers questions, names it for the agent_threads
// row, and hands the bare model to the scheduler for nudge drafts. Without a key the echo runner stands in, and the warning
// says so: an operator reading the log must be able to tell "the model is
// broken" from "there is no model".
func buildRunner(cfg config.Config, reader *readstore.Store, events *logclient.Client) (string, discord.Runner, model.Model, error) {
	if !cfg.ModelEnabled() {
		events.LogAsync(logclient.Warning, "Model unset", nil)
		log.Println("warning: GEMINI_API_KEY unset — every answer is an echo")
		return "echo", discord.EchoRunner{}, nil, nil
	}
	answering, err := gemini.New(context.Background(), cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		events.LogAsync(logclient.Error, "Model unavailable", map[string]any{"error": err.Error()})
		return "", nil, nil, fmt.Errorf("build model: %w", err)
	}
	// A nil reader is passed on purpose: tools.All leaves out the SQL tools
	// rather than building tools with nothing to read.
	var source tools.Reader
	if reader != nil {
		source = reader
	}
	available := tools.All(source, cfg.StatusURL, cfg.CalendarURL)
	loop := agent.New(answering, available, agent.Options{Timeout: cfg.AgentTimeout})
	names := make([]string, 0, len(available))
	for _, tool := range available {
		names = append(names, tool.Spec().Name)
	}
	events.LogAsync(logclient.Info, "Model ready", map[string]any{
		"model": loop.Model(), "tools": names,
	})
	return loop.Model(), agent.NewRunner(loop), answering, nil
}

func connectStore(databaseURL string) (*store.Store, error) {
	deadline := time.Now().Add(2 * time.Minute)
	for attempt := 1; ; attempt++ {
		sprinterStore, err := store.New(context.Background(), databaseURL)
		if err == nil {
			return sprinterStore, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		log.Printf("database not ready (attempt %d): %v; retrying in 3s", attempt, err)
		time.Sleep(3 * time.Second)
	}
}

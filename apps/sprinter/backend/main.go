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

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/agent"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/auth"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/calendar"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/config"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/discord"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model/gemini"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/readstore"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/scheduler"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/server"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/tools"
)

func main() {
	_ = godotenv.Load("../.env")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	// The event client is built before anything can block, so a warning still
	// reaches Logger when the database never comes up and the process dies in
	// connectStore.
	events := logclient.NewFromEnv("sprinter")
	if !events.Enabled() {
		log.Println("warning: LOGGER_INGEST_TOKEN unset — structured logging is disabled")
	}

	sprinterStore, err := connectStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
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
			log.Fatalf("connect read database: %v", err)
		}
		defer reader.Close()
	} else {
		events.LogAsync(logclient.Warning, "Read database unset", nil)
	}

	modelName, runner, answering := buildRunner(cfg, reader, events)

	var bot *discord.Bot
	if !cfg.DiscordEnabled() {
		events.LogAsync(logclient.Warning, "Discord token unset", nil)
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
			log.Fatalf("create Discord session: %v", err)
		}
		if err := bot.Open(); err != nil {
			events.LogAsync(logclient.Error, "Discord session failed", map[string]any{
				"stage": "open", "error": err.Error(),
			})
			log.Fatalf("open Discord session: %v", err)
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
			log.Fatalf("load location: %v", err)
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

	authClient := auth.NewClient(cfg.OAuthManagerURL)
	app := server.New(cfg, sprinterStore, authClient, events, bot.Connected)
	address := ":" + cfg.Port
	events.LogAsync(logclient.Info, "Sprinter started", map[string]any{
		"port": cfg.Port, "discord": cfg.DiscordEnabled(),
	})

	go func() {
		log.Printf("Sprinter admin API listening on %s", address)
		if err := app.Listen(address, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
	shutdownLogCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := events.Log(shutdownLogCtx, logclient.Info, "Sprinter stopping", nil); err != nil {
		log.Printf("structured shutdown log: %v", err)
	}
	cancel()
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// buildRunner picks what answers questions, names it for the agent_threads
// row, and hands the bare model to the scheduler for nudge drafts. Without a key the echo runner stands in, and the warning
// says so: an operator reading the log must be able to tell "the model is
// broken" from "there is no model".
func buildRunner(cfg config.Config, reader *readstore.Store, events *logclient.Client) (string, discord.Runner, model.Model) {
	if !cfg.ModelEnabled() {
		events.LogAsync(logclient.Warning, "Model unset", nil)
		log.Println("warning: GEMINI_API_KEY unset — every answer is an echo")
		return "echo", discord.EchoRunner{}, nil
	}
	answering, err := gemini.New(context.Background(), cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		events.LogAsync(logclient.Error, "Model unavailable", map[string]any{"error": err.Error()})
		log.Fatalf("build model: %v", err)
	}
	// A nil reader is passed on purpose: tools.All leaves out the SQL tools
	// rather than building tools with nothing to read.
	var source tools.Reader
	if reader != nil {
		source = reader
	}
	available := tools.All(source, cfg.LoggerURL, cfg.CalendarURL)
	loop := agent.New(answering, available, agent.Options{Timeout: cfg.AgentTimeout})
	names := make([]string, 0, len(available))
	for _, tool := range available {
		names = append(names, tool.Spec().Name)
	}
	events.LogAsync(logclient.Info, "Model ready", map[string]any{
		"model": loop.Model(), "tools": names,
	})
	return loop.Model(), agent.NewRunner(loop), answering
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

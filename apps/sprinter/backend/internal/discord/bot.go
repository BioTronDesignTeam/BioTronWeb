// Package discord is Sprinter's gateway session and its slash commands.
//
// Two commands exist: `/agent` answers once in the channel, `/agent-thread`
// answers and then opens a thread to carry the conversation. Their names are
// the guard subjects, so a command and the guard that admits it cannot drift
// apart.
package discord

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/BioTronDesignTeam/BioTronWeb/go/logclient"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// discordTimeout bounds one call to Discord. An interaction token is valid for
// fifteen minutes, but a call that hangs holds a goroutine and the person who
// ran the command sees nothing, so every call gets a deadline.
const discordTimeout = 15 * time.Second

// The command names, which are also the guard subjects.
const (
	commandAgent       = store.SubjectAgent
	commandAgentThread = store.SubjectAgentThread
)

// Store is the part of the database the bot uses.
type Store interface {
	GetGuard(ctx context.Context, subject string) (store.Guard, error)
	CreateThread(ctx context.Context, thread store.Thread) (store.Thread, error)
	AppendMessage(ctx context.Context, threadID string, sequence int, role string, content json.RawMessage) (store.Message, error)
}

type Options struct {
	// GuildID is the one guild commands are registered in. Guild commands
	// appear at once; global ones take up to an hour to propagate.
	GuildID string
	// Model names the answering model on the agent_threads row.
	Model  string
	Runner Runner
}

type Bot struct {
	session *discordgo.Session
	store   Store
	events  *logclient.Client
	options Options
	// connected is read by /health from another goroutine, and the gateway
	// drops and resumes while the process stays up.
	connected atomic.Bool
}

func New(token string, sprinterStore Store, events *logclient.Client, options Options) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	if options.Runner == nil {
		options.Runner = EchoRunner{}
	}
	if options.Model == "" {
		options.Model = "echo"
	}
	// Guilds and GuildMessages are the baseline. MessageContent is privileged
	// and deliberate: a thread follow-up is a plain message, not a command, so
	// without it the bot would read empty strings. Only messages in threads the
	// bot itself opened are ever handled.
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent

	bot := &Bot{session: session, store: sprinterStore, events: events, options: options}

	// Handlers are registered before Open so the gateway's first Ready cannot
	// arrive before anyone is listening for it.
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onDisconnect)
	session.AddHandler(bot.onResumed)
	session.AddHandler(bot.onInteraction)
	session.AddHandler(bot.onMessage)
	return bot, nil
}

func (b *Bot) Open() error {
	return b.session.Open()
}

func (b *Bot) Close() error {
	return b.session.Close()
}

// Connected reports whether the gateway session is up, for /health.
func (b *Bot) Connected() bool {
	return b != nil && b.connected.Load()
}

func (b *Bot) onReady(session *discordgo.Session, ready *discordgo.Ready) {
	b.connected.Store(true)
	b.events.LogAsync(logclient.Info, "Discord connected", map[string]any{
		"user":   ready.User.Username,
		"guilds": len(ready.Guilds),
	})
	b.registerCommands(session, ready.User.ID)
}

func (b *Bot) onDisconnect(_ *discordgo.Session, _ *discordgo.Disconnect) {
	b.connected.Store(false)
	b.events.LogAsync(logclient.Warning, "Discord disconnected", nil)
}

func (b *Bot) onResumed(_ *discordgo.Session, _ *discordgo.Resumed) {
	b.connected.Store(true)
	b.events.LogAsync(logclient.Info, "Discord resumed", nil)
}

// onMessage is deliberately empty. The Message Content intent is on so that a
// thread follow-up can be a plain message; the handler that reads those, and
// only in threads the bot opened, is the agent loop's, not this file's.
func (b *Bot) onMessage(_ *discordgo.Session, _ *discordgo.MessageCreate) {}

// registerCommands replaces the guild's commands with exactly these two. Bulk
// overwrite is the only call that also removes a command we no longer publish,
// so a renamed command cannot linger in the picker.
func (b *Bot) registerCommands(session *discordgo.Session, applicationID string) {
	question := []*discordgo.ApplicationCommandOption{{
		Type:        discordgo.ApplicationCommandOptionString,
		Name:        "question",
		Description: "What to ask.",
		Required:    true,
	}}
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        commandAgent,
			Description: "Ask Sprinter a question and get one answer here.",
			Options:     question,
		},
		{
			Name:        commandAgentThread,
			Description: "Ask Sprinter a question and open a thread to keep going.",
			Options:     question,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	defer cancel()
	registered, err := session.ApplicationCommandBulkOverwrite(
		applicationID, b.options.GuildID, commands, discordgo.WithContext(ctx))
	if err != nil {
		log.Printf("register commands: %v", err)
		b.events.LogAsync(logclient.Error, "Command registration failed", map[string]any{
			"guild_id": b.options.GuildID,
			"error":    err.Error(),
		})
		return
	}
	b.events.LogAsync(logclient.Info, "Commands registered", map[string]any{
		"guild_id": b.options.GuildID,
		"count":    len(registered),
	})
}

func (b *Bot) onInteraction(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := interaction.ApplicationCommandData()
	if data.Name != commandAgent && data.Name != commandAgentThread {
		return
	}
	// Discord gives three seconds to acknowledge, so the gate and the answer
	// both run on their own goroutine rather than on the gateway's.
	go b.handleCommand(session, interaction, data)
}

func (b *Bot) handleCommand(session *discordgo.Session, interaction *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	subject := data.Name
	question := optionString(data.Options, "question")

	storeCtx, cancelStore := context.WithTimeout(context.Background(), store.Timeout)
	guard, err := b.store.GetGuard(storeCtx, subject)
	cancelStore()
	switch {
	case errors.Is(err, store.ErrNotFound):
		// No guard is a refusal, not an open door. A command nobody has
		// configured must not run.
		b.refuse(session, interaction, subject, reasonNoGuard)
		return
	case err != nil:
		log.Printf("read guard %q: %v", subject, err)
		b.refuse(session, interaction, subject, reasonUnavailable)
		return
	}
	if ok, why := allowed(guard, interaction); !ok {
		b.refuse(session, interaction, subject, why)
		return
	}

	if err := b.acknowledge(session, interaction); err != nil {
		log.Printf("acknowledge %q: %v", subject, err)
		b.events.LogAsync(logclient.Error, "Question failed", map[string]any{
			"subject": subject, "stage": "defer", "error": err.Error(),
		})
		return
	}

	started := time.Now()
	answerCtx, cancelAnswer := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelAnswer()
	answer, err := b.options.Runner.Answer(answerCtx, Request{
		Subject:   subject,
		GuildID:   interaction.GuildID,
		ChannelID: interaction.ChannelID,
		UserID:    userID(interaction),
		Question:  question,
	})
	if err != nil {
		log.Printf("answer %q: %v", subject, err)
		b.events.LogAsync(logclient.Error, "Question failed", map[string]any{
			"subject": subject, "stage": "answer",
			"user_id": userID(interaction), "channel_id": interaction.ChannelID,
			"duration_ms": time.Since(started).Milliseconds(), "error": err.Error(),
		})
		b.followup(session, interaction, "Sprinter could not answer that. Try again shortly.")
		return
	}

	first := b.post(session, interaction, answer)
	if first != nil && subject == commandAgentThread {
		b.openThread(session, interaction, first, question, answer)
	}
	b.events.LogAsync(logclient.Info, "Question answered", map[string]any{
		"subject":     subject,
		"user_id":     userID(interaction),
		"channel_id":  interaction.ChannelID,
		"duration_ms": time.Since(started).Milliseconds(),
	})
}

// post sends the answer as one or more follow-ups and returns the first
// message, which is the one a thread hangs off.
func (b *Bot) post(session *discordgo.Session, interaction *discordgo.InteractionCreate, answer string) *discordgo.Message {
	parts := splitMessage(answer, messageLimit)
	if len(parts) == 0 {
		parts = []string{"(no answer)"}
	}
	var first *discordgo.Message
	for _, part := range parts {
		message := b.followup(session, interaction, part)
		if message == nil {
			return first
		}
		if first == nil {
			first = message
		}
	}
	return first
}

// openThread hangs a thread off the answer and stores the first two turns, so
// the agent loop can later replay the conversation from the database rather
// than from Discord's history.
func (b *Bot) openThread(session *discordgo.Session, interaction *discordgo.InteractionCreate, message *discordgo.Message, question, answer string) {
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	// 1440 minutes is a day: long enough for a conversation to resume the next
	// morning, short enough that the channel list does not fill with threads.
	thread, err := session.MessageThreadStart(
		message.ChannelID, message.ID, threadName(question), 1440, discordgo.WithContext(ctx))
	cancel()
	if err != nil {
		log.Printf("start thread: %v", err)
		b.events.LogAsync(logclient.Error, "Thread creation failed", map[string]any{
			"channel_id": message.ChannelID, "error": err.Error(),
		})
		return
	}

	storeCtx, cancelStore := context.WithTimeout(context.Background(), store.Timeout)
	defer cancelStore()
	if _, err := b.store.CreateThread(storeCtx, store.Thread{
		ThreadID:  thread.ID,
		GuildID:   interaction.GuildID,
		ChannelID: message.ChannelID,
		OpenerID:  userID(interaction),
		Model:     b.options.Model,
		TurnCount: 1,
	}); err != nil {
		log.Printf("record thread: %v", err)
		b.events.LogAsync(logclient.Error, "Thread record failed", map[string]any{
			"thread_id": thread.ID, "error": err.Error(),
		})
		return
	}
	b.appendTurn(storeCtx, thread.ID, 1, model.Message{Role: model.RoleUser, Text: question})
	b.appendTurn(storeCtx, thread.ID, 2, model.Message{Role: model.RoleAssistant, Text: answer})
	b.events.LogAsync(logclient.Info, "Thread opened", map[string]any{
		"thread_id": thread.ID, "channel_id": message.ChannelID,
		"opener_id": userID(interaction),
	})
}

// appendTurn stores one transcript row. The content is the JSON encoding of a
// model.Message, so a vendor growing a field does not need a migration.
func (b *Bot) appendTurn(ctx context.Context, threadID string, sequence int, message model.Message) {
	content, err := json.Marshal(message)
	if err != nil {
		log.Printf("encode transcript turn: %v", err)
		return
	}
	if _, err := b.store.AppendMessage(ctx, threadID, sequence, string(message.Role), content); err != nil {
		log.Printf("append transcript turn: %v", err)
		b.events.LogAsync(logclient.Error, "Transcript write failed", map[string]any{
			"thread_id": threadID, "sequence": sequence, "error": err.Error(),
		})
	}
}

func (b *Bot) acknowledge(session *discordgo.Session, interaction *discordgo.InteractionCreate) error {
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	defer cancel()
	return session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}, discordgo.WithContext(ctx))
}

func (b *Bot) followup(session *discordgo.Session, interaction *discordgo.InteractionCreate, content string) *discordgo.Message {
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	defer cancel()
	message, err := session.FollowupMessageCreate(interaction.Interaction, true,
		&discordgo.WebhookParams{Content: content}, discordgo.WithContext(ctx))
	if err != nil {
		log.Printf("follow-up message: %v", err)
		b.events.LogAsync(logclient.Error, "Follow-up failed", map[string]any{
			"channel_id": interaction.ChannelID, "error": err.Error(),
		})
		return nil
	}
	return message
}

// refuse answers only the person who ran the command, and logs why. The
// question itself is never logged: a refused command is often a mistake, and
// the audit trail does not need its text to be useful.
func (b *Bot) refuse(session *discordgo.Session, interaction *discordgo.InteractionCreate, subject string, why reason) {
	b.events.LogAsync(logclient.Warning, "Command refused", map[string]any{
		"subject":    subject,
		"reason":     string(why),
		"user_id":    userID(interaction),
		"guild_id":   interaction.GuildID,
		"channel_id": interaction.ChannelID,
	})
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	defer cancel()
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: refusals[why],
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}, discordgo.WithContext(ctx))
	if err != nil {
		log.Printf("refuse %q: %v", subject, err)
	}
}

func optionString(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, option := range options {
		if option.Name == name && option.Type == discordgo.ApplicationCommandOptionString {
			return option.StringValue()
		}
	}
	return ""
}

// userID reads the caller from wherever Discord put them: Member in a guild,
// User in a direct message.
func userID(interaction *discordgo.InteractionCreate) string {
	if interaction.Member != nil && interaction.Member.User != nil {
		return interaction.Member.User.ID
	}
	if interaction.User != nil {
		return interaction.User.ID
	}
	return ""
}

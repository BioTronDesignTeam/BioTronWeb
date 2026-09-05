// Package discord is Sprinter's gateway session and its slash commands.
//
// Two commands exist: `/agent` answers once in the channel, `/agent-thread`
// answers and then opens a thread to carry the conversation. Their names are
// the guard subjects, so a command and the guard that admits it cannot drift
// apart. A message in one of those threads continues it; a message anywhere
// else is ignored, which is what keeps the Message Content intent contained.
package discord

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
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

// answerTimeout bounds one whole question, above whatever deadline the runner
// keeps for itself. It is the last thing that frees a concurrency slot.
const answerTimeout = 5 * time.Minute

// DefaultMaxThreadTurns is how many questions one thread answers. Every turn
// is replayed to the model, so an old thread costs more per answer than a new
// one; forty is where a conversation is better restarted than continued.
const DefaultMaxThreadTurns = 40

// typingInterval refreshes the typing indicator, which Discord clears after
// about ten seconds.
const typingInterval = 8 * time.Second

// The command names, which are also the guard subjects.
const (
	commandAgent       = store.SubjectAgent
	commandAgentThread = store.SubjectAgentThread
)

// Store is the part of the database the bot uses.
type Store interface {
	GetGuard(ctx context.Context, subject string) (store.Guard, error)
	CreateThread(ctx context.Context, thread store.Thread) (store.Thread, error)
	GetThread(ctx context.Context, threadID string) (store.Thread, error)
	TouchThread(ctx context.Context, threadID string) (store.Thread, error)
	AppendMessage(ctx context.Context, threadID string, sequence int, role string, content json.RawMessage) (store.Message, error)
	ListMessages(ctx context.Context, threadID string) ([]store.Message, error)
}

type Options struct {
	// GuildID is the one guild commands are registered in. Guild commands
	// appear at once; global ones take up to an hour to propagate.
	GuildID string
	// Model names the answering model on the agent_threads row.
	Model  string
	Runner Runner
	// MaxThreadTurns caps one thread's conversation.
	MaxThreadTurns int
}

type Bot struct {
	session *discordgo.Session
	store   Store
	events  *logclient.Client
	options Options
	limits  *limiter
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
	if options.MaxThreadTurns <= 0 {
		options.MaxThreadTurns = DefaultMaxThreadTurns
	}
	// Guilds and GuildMessages are the baseline. MessageContent is privileged
	// and deliberate: a thread follow-up is a plain message, not a command, so
	// without it the bot would read empty strings. Only messages in threads the
	// bot itself opened are ever handled.
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent

	bot := &Bot{
		session: session, store: sprinterStore, events: events,
		options: options, limits: newLimiter(maxConcurrent),
	}

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
	asker := userID(interaction)

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

	// The concurrency check comes after the gate and before the deferred
	// reply, so a refused person gets one ephemeral sentence rather than a
	// "thinking" message that never resolves.
	release, busy := b.limits.acquire(asker)
	if release == nil {
		b.ephemeral(session, interaction, busy)
		return
	}
	defer release()

	if err := b.acknowledge(session, interaction); err != nil {
		log.Printf("acknowledge %q: %v", subject, err)
		b.events.LogAsync(logclient.Error, "Question failed", map[string]any{
			"subject": subject, "stage": "defer", "error": err.Error(),
		})
		return
	}

	started := time.Now()
	answerCtx, cancelAnswer := context.WithTimeout(context.Background(), answerTimeout)
	defer cancelAnswer()
	reply, err := b.options.Runner.Answer(answerCtx, Request{
		Subject:   subject,
		GuildID:   interaction.GuildID,
		ChannelID: interaction.ChannelID,
		UserID:    asker,
		Question:  question,
	})
	if err != nil {
		log.Printf("answer %q: %v", subject, err)
		b.events.LogAsync(logclient.Error, "Question failed", map[string]any{
			"subject": subject, "stage": "answer",
			"user_id": asker, "channel_id": interaction.ChannelID,
			"duration_ms": time.Since(started).Milliseconds(), "error": err.Error(),
		})
		b.followup(session, interaction, "Sprinter could not answer that. Try again shortly.")
		return
	}

	first := b.post(session, interaction, reply.Text)
	if first != nil && subject == commandAgentThread {
		b.openThread(session, interaction, first, question, reply)
	}
	payload := map[string]any{
		"subject":     subject,
		"user_id":     asker,
		"channel_id":  interaction.ChannelID,
		"duration_ms": time.Since(started).Milliseconds(),
	}
	addCost(payload, reply)
	b.events.LogAsync(logclient.Info, "Question answered", payload)
}

// onMessage carries thread follow-ups. Everything that is not a person typing
// in a thread the bot opened is dropped here, before any work: that is the
// whole justification for holding the Message Content intent.
func (b *Bot) onMessage(session *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author == nil || message.Author.Bot || message.GuildID == "" {
		return
	}
	if strings.TrimSpace(message.Content) == "" {
		return
	}
	// A plain message is the only kind that can be a question. A join notice
	// or a pin has a type of its own and is not one.
	if message.Type != discordgo.MessageTypeDefault && message.Type != discordgo.MessageTypeReply {
		return
	}
	go b.handleThreadMessage(session, message)
}

func (b *Bot) handleThreadMessage(session *discordgo.Session, message *discordgo.MessageCreate) {
	storeCtx, cancelStore := context.WithTimeout(context.Background(), store.Timeout)
	thread, err := b.store.GetThread(storeCtx, message.ChannelID)
	cancelStore()
	switch {
	case errors.Is(err, store.ErrNotFound):
		// Not one of our threads. This is the common case for every message in
		// the guild, so it is silent.
		return
	case err != nil:
		log.Printf("read thread %q: %v", message.ChannelID, err)
		return
	}

	asker := message.Author.ID
	if !b.threadAllowed(session, message, thread) {
		return
	}
	if thread.TurnCount >= b.options.MaxThreadTurns {
		b.say(session, message.ChannelID,
			"This thread has reached its limit. Start a new one with /agent-thread.")
		return
	}

	release, busy := b.limits.acquire(asker)
	if release == nil {
		b.say(session, message.ChannelID, busy)
		return
	}
	defer release()

	// A thread answer has no deferred reply to stand in for it, so the typing
	// indicator is the only sign the bot is working.
	stopTyping := b.typing(session, message.ChannelID)
	defer stopTyping()

	started := time.Now()
	historyCtx, cancelHistory := context.WithTimeout(context.Background(), store.Timeout)
	rows, err := b.store.ListMessages(historyCtx, thread.ThreadID)
	cancelHistory()
	if err != nil {
		log.Printf("read transcript %q: %v", thread.ThreadID, err)
		b.threadFailed(session, message, thread, "history", started, err)
		return
	}
	history, next := decodeTranscript(rows)

	answerCtx, cancelAnswer := context.WithTimeout(context.Background(), answerTimeout)
	reply, err := b.options.Runner.Continue(answerCtx, Request{
		Subject:   commandAgentThread,
		GuildID:   message.GuildID,
		ChannelID: thread.ChannelID,
		ThreadID:  thread.ThreadID,
		UserID:    asker,
		Question:  message.Content,
	}, history)
	cancelAnswer()
	if err != nil {
		log.Printf("continue thread %q: %v", thread.ThreadID, err)
		b.threadFailed(session, message, thread, "answer", started, err)
		return
	}

	// A runner that answered nothing — a rate-limited model, say — leaves no
	// turns. The person still reads the sentence, but the thread must not spend
	// a turn or store half an exchange it could never replay.
	if len(reply.Messages) > 0 {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), store.Timeout)
		for _, turn := range reply.Messages {
			b.appendTurn(writeCtx, thread.ThreadID, next, turn)
			next++
		}
		if _, err := b.store.TouchThread(writeCtx, thread.ThreadID); err != nil {
			log.Printf("touch thread %q: %v", thread.ThreadID, err)
		}
		cancelWrite()
	}

	stopTyping()
	for _, part := range splitMessage(reply.Text, messageLimit) {
		b.say(session, message.ChannelID, part)
	}
	payload := map[string]any{
		"subject":     commandAgentThread,
		"thread_id":   thread.ThreadID,
		"user_id":     asker,
		"duration_ms": time.Since(started).Milliseconds(),
	}
	addCost(payload, reply)
	b.events.LogAsync(logclient.Info, "Question answered", payload)
}

// threadAllowed re-runs the `agent-thread` guard for a follow-up. The guard
// can change, and a role can be taken away, between the first question and the
// tenth; a thread must not be a way to keep an access somebody has lost.
func (b *Bot) threadAllowed(session *discordgo.Session, message *discordgo.MessageCreate, thread store.Thread) bool {
	ctx, cancel := context.WithTimeout(context.Background(), store.Timeout)
	guard, err := b.store.GetGuard(ctx, commandAgentThread)
	cancel()
	if err != nil {
		why := reasonUnavailable
		if errors.Is(err, store.ErrNotFound) {
			why = reasonNoGuard
		} else {
			log.Printf("read guard %q: %v", commandAgentThread, err)
		}
		b.refuseInThread(session, message, thread, why)
		return false
	}
	var roles []string
	if message.Member != nil {
		roles = message.Member.Roles
	}
	// The guard's channel list names channels, not threads, so the thread's
	// parent is what gets checked.
	ok, why := allowedIn(guard, message.GuildID, thread.ChannelID, roles, message.Member != nil)
	if !ok {
		b.refuseInThread(session, message, thread, why)
	}
	return ok
}

func (b *Bot) refuseInThread(session *discordgo.Session, message *discordgo.MessageCreate, thread store.Thread, why reason) {
	b.events.LogAsync(logclient.Warning, "Command refused", map[string]any{
		"subject":    commandAgentThread,
		"reason":     string(why),
		"user_id":    message.Author.ID,
		"guild_id":   message.GuildID,
		"channel_id": thread.ChannelID,
		"thread_id":  thread.ThreadID,
	})
	b.say(session, message.ChannelID, refusals[why])
}

func (b *Bot) threadFailed(session *discordgo.Session, message *discordgo.MessageCreate, thread store.Thread, stage string, started time.Time, err error) {
	b.events.LogAsync(logclient.Error, "Question failed", map[string]any{
		"subject": commandAgentThread, "stage": stage,
		"thread_id": thread.ThreadID, "user_id": message.Author.ID,
		"duration_ms": time.Since(started).Milliseconds(), "error": err.Error(),
	})
	b.say(session, message.ChannelID, "Sprinter could not answer that. Try again shortly.")
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

// openThread hangs a thread off the answer and stores the turns the runner
// built, so a follow-up replays the conversation from the database rather than
// from Discord's history. The runner's transcript is stored rather than a
// question-and-answer pair, because the tool calls in between are what let the
// model continue the search instead of starting it again.
func (b *Bot) openThread(session *discordgo.Session, interaction *discordgo.InteractionCreate, message *discordgo.Message, question string, reply Reply) {
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
	turns := reply.Messages
	if len(turns) == 0 {
		// A runner that returned no transcript still leaves a usable thread:
		// the question and the answer are what a follow-up needs at minimum.
		turns = []model.Message{
			{Role: model.RoleUser, Text: question},
			{Role: model.RoleAssistant, Text: reply.Text},
		}
	}
	for i, turn := range turns {
		b.appendTurn(storeCtx, thread.ID, i+1, turn)
	}
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

// decodeTranscript turns stored rows back into model turns and reports the
// next free sequence number. A row that will not decode is skipped rather than
// fatal: one corrupt turn must not close a conversation.
func decodeTranscript(rows []store.Message) ([]model.Message, int) {
	history := make([]model.Message, 0, len(rows))
	next := 1
	for _, row := range rows {
		if row.Sequence >= next {
			next = row.Sequence + 1
		}
		var turn model.Message
		if err := json.Unmarshal(row.Content, &turn); err != nil {
			log.Printf("decode transcript turn %d of %s: %v", row.Sequence, row.ThreadID, err)
			continue
		}
		history = append(history, turn)
	}
	return history, next
}

// typing keeps the "Sprinter is typing" indicator alive until the returned
// function is called. Discord clears it after about ten seconds, so it has to
// be refreshed rather than set once.
func (b *Bot) typing(session *discordgo.Session, channelID string) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(typingInterval)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
			// A failed indicator is cosmetic. The answer still arrives.
			_ = session.ChannelTyping(channelID, discordgo.WithContext(ctx))
			cancel()
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (b *Bot) say(session *discordgo.Session, channelID, content string) {
	if content == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	defer cancel()
	if _, err := session.ChannelMessageSend(channelID, content, discordgo.WithContext(ctx)); err != nil {
		log.Printf("send message: %v", err)
		b.events.LogAsync(logclient.Error, "Follow-up failed", map[string]any{
			"channel_id": channelID, "error": err.Error(),
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
	b.ephemeral(session, interaction, refusals[why])
}

// ephemeral answers the person who ran the command and nobody else.
func (b *Bot) ephemeral(session *discordgo.Session, interaction *discordgo.InteractionCreate, content string) {
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	defer cancel()
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}, discordgo.WithContext(ctx))
	if err != nil {
		log.Printf("ephemeral reply: %v", err)
	}
}

// addCost records what a question spent. The question and the answer are never
// logged; what it cost and which tools ran are, because that is what an
// operator needs to see a runaway.
func addCost(payload map[string]any, reply Reply) {
	if len(reply.Tools) > 0 {
		payload["tools"] = reply.Tools
	}
	if reply.InputTokens > 0 || reply.OutputTokens > 0 {
		payload["input_tokens"] = reply.InputTokens
		payload["output_tokens"] = reply.OutputTokens
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

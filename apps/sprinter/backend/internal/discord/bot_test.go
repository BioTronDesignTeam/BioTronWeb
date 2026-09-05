package discord

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// call is one thing the bot did to Discord, in the order it did it. The order
// is the point: the acknowledgement has to come before the guard read.
type call struct {
	kind    string // acknowledge, edit, followup, thread, send, typing
	channel string
	content string
}

// fakeAPI records every write and can fail any one of them.
type fakeAPI struct {
	mu     sync.Mutex
	calls  []call
	parent map[string]string
	// threadErr fails StartThread; followupErr fails Followup.
	threadErr, followupErr error
}

func newFakeAPI() *fakeAPI { return &fakeAPI{parent: map[string]string{}} }

func (f *fakeAPI) record(c call) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
}

func (f *fakeAPI) kinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var kinds []string
	for _, c := range f.calls {
		kinds = append(kinds, c.kind)
	}
	return kinds
}

func (f *fakeAPI) only(kind string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []call
	for _, c := range f.calls {
		if c.kind == kind {
			found = append(found, c)
		}
	}
	return found
}

func (f *fakeAPI) Acknowledge(*discordgo.Interaction) error {
	f.record(call{kind: "acknowledge"})
	return nil
}

func (f *fakeAPI) EditResponse(_ *discordgo.Interaction, content string) error {
	f.record(call{kind: "edit", content: content})
	return nil
}

func (f *fakeAPI) Followup(interaction *discordgo.Interaction, content string) (*discordgo.Message, error) {
	if f.followupErr != nil {
		return nil, f.followupErr
	}
	f.record(call{kind: "followup", channel: interaction.ChannelID, content: content})
	return &discordgo.Message{ID: "message-1", ChannelID: interaction.ChannelID}, nil
}

func (f *fakeAPI) StartThread(channelID, _, name string) (string, error) {
	if f.threadErr != nil {
		return "", f.threadErr
	}
	f.record(call{kind: "thread", channel: channelID, content: name})
	return "thread-1", nil
}

func (f *fakeAPI) Send(channelID, content string) error {
	f.record(call{kind: "send", channel: channelID, content: content})
	return nil
}

func (f *fakeAPI) Typing(channelID string) {
	f.record(call{kind: "typing", channel: channelID})
}

func (f *fakeAPI) ParentChannelID(channelID string) string { return f.parent[channelID] }

// fakeStore answers the two reads handleCommand makes and records the writes.
type fakeStore struct {
	guard    store.Guard
	guardErr error
	// guardDelay stands in for a slow database. The acknowledgement has to
	// have gone out before this is waited on.
	guardDelay time.Duration

	mu      sync.Mutex
	threads []store.Thread
	turns   []model.Message
}

func (f *fakeStore) GetGuard(context.Context, string) (store.Guard, error) {
	if f.guardDelay > 0 {
		time.Sleep(f.guardDelay)
	}
	return f.guard, f.guardErr
}

func (f *fakeStore) CreateThread(_ context.Context, thread store.Thread) (store.Thread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.threads = append(f.threads, thread)
	return thread, nil
}

func (f *fakeStore) GetThread(context.Context, string) (store.Thread, error) {
	return store.Thread{}, store.ErrNotFound
}

func (f *fakeStore) TouchThread(_ context.Context, threadID string) (store.Thread, error) {
	return store.Thread{ThreadID: threadID}, nil
}

func (f *fakeStore) AppendMessage(_ context.Context, threadID string, sequence int, _ string, content json.RawMessage) (store.Message, error) {
	var turn model.Message
	if err := json.Unmarshal(content, &turn); err != nil {
		return store.Message{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, turn)
	return store.Message{ThreadID: threadID, Sequence: sequence}, nil
}

func (f *fakeStore) ListMessages(context.Context, string) ([]store.Message, error) {
	return nil, nil
}

// scriptedRunner answers with a fixed Reply.
type scriptedRunner struct {
	reply Reply
	err   error
}

func (r scriptedRunner) Answer(context.Context, Request) (Reply, error) {
	return r.reply, r.err
}

func (r scriptedRunner) Continue(context.Context, Request, []model.Message) (Reply, error) {
	return r.reply, r.err
}

func testBot(api *fakeAPI, sprinterStore Store, runner Runner) *Bot {
	return &Bot{
		api: api, store: sprinterStore,
		options: Options{Runner: runner, Model: "fake", MaxThreadTurns: DefaultMaxThreadTurns},
		limits:  newLimiter(maxConcurrent),
	}
}

func command(name, channelID string, roles ...string) (*discordgo.InteractionCreate, discordgo.ApplicationCommandInteractionData) {
	create := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		GuildID:   guild,
		ChannelID: channelID,
		Member: &discordgo.Member{
			Roles: roles,
			User:  &discordgo.User{ID: "400000000000000001"},
		},
	}}
	data := discordgo.ApplicationCommandInteractionData{
		Name: name,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{{
			Name: "question", Type: discordgo.ApplicationCommandOptionString,
			Value: "how is the platform?",
		}},
	}
	return create, data
}

func openGuard(subject string) store.Guard {
	return store.Guard{Subject: subject, GuildID: guild, RoleIDs: []string{leads}}
}

// Discord closes an interaction three seconds after it arrives. The guard is
// a database read, so it must not stand between the command and its
// acknowledgement: a slow database used to cost the person their answer with
// nothing said at all.
func TestTheAcknowledgementComesBeforeTheGuardRead(t *testing.T) {
	api := newFakeAPI()
	sprinterStore := &fakeStore{guard: openGuard(commandAgent), guardDelay: 20 * time.Millisecond}
	bot := testBot(api, sprinterStore, scriptedRunner{reply: Reply{
		Text:     "Everything is operational.",
		Messages: []model.Message{{Role: model.RoleAssistant, Text: "Everything is operational."}},
	}})

	interaction, data := command(commandAgent, general, leads)
	bot.handleCommand(interaction, data)

	kinds := api.kinds()
	if len(kinds) == 0 || kinds[0] != "acknowledge" {
		t.Fatalf("calls = %v, want the acknowledgement first", kinds)
	}
}

// A refusal now edits the deferred reply, because the reply is already out by
// the time the guard says no.
func TestARefusalEditsTheDeferredReply(t *testing.T) {
	cases := map[string]struct {
		guard    store.Guard
		guardErr error
		roles    []string
		want     reason
	}{
		"no guard configured": {guardErr: store.ErrNotFound, roles: []string{leads}, want: reasonNoGuard},
		"the database is down": {
			guardErr: errors.New("connection refused"), roles: []string{leads}, want: reasonUnavailable,
		},
		"the wrong role": {guard: openGuard(commandAgent), roles: []string{guests}, want: reasonWrongRole},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			api := newFakeAPI()
			sprinterStore := &fakeStore{guard: testCase.guard, guardErr: testCase.guardErr}
			bot := testBot(api, sprinterStore, scriptedRunner{})

			interaction, data := command(commandAgent, general, testCase.roles...)
			bot.handleCommand(interaction, data)

			if kinds := api.kinds(); len(kinds) != 2 || kinds[0] != "acknowledge" || kinds[1] != "edit" {
				t.Fatalf("calls = %v, want acknowledge then edit", kinds)
			}
			if got := api.only("edit")[0].content; got != refusals[testCase.want] {
				t.Fatalf("refusal = %q, want %q", got, refusals[testCase.want])
			}
		})
	}
}

// A guard that names channels has to admit a question asked in a thread under
// one of them. The thread has an id of its own that no guard could list.
func TestACommandInAThreadIsGatedOnTheParentChannel(t *testing.T) {
	guard := store.Guard{
		Subject: commandAgent, GuildID: guild,
		RoleIDs: []string{leads}, ChannelIDs: []string{general},
	}
	answered := Reply{
		Text:     "Everything is operational.",
		Messages: []model.Message{{Role: model.RoleAssistant, Text: "Everything is operational."}},
	}

	api := newFakeAPI()
	api.parent[thread] = general
	bot := testBot(api, &fakeStore{guard: guard}, scriptedRunner{reply: answered})
	interaction, data := command(commandAgent, thread, leads)
	bot.handleCommand(interaction, data)
	if len(api.only("followup")) != 1 {
		t.Fatalf("a thread under the guarded channel was refused: %v", api.kinds())
	}

	// A thread under a channel the guard does not name is still refused.
	api = newFakeAPI()
	api.parent[thread] = offtop
	bot = testBot(api, &fakeStore{guard: guard}, scriptedRunner{reply: answered})
	interaction, data = command(commandAgent, thread, leads)
	bot.handleCommand(interaction, data)
	edits := api.only("edit")
	if len(edits) != 1 || edits[0].content != refusals[reasonWrongChanel] {
		t.Fatalf("calls = %v, edits = %+v", api.kinds(), edits)
	}
}

// A runner that answered with no transcript — rate limited, or failed — has
// nothing a follow-up could replay. The answer still posts; the thread must
// not be opened, and the bot must not invent turns the model never produced.
func TestNoTranscriptMeansNoThread(t *testing.T) {
	api := newFakeAPI()
	sprinterStore := &fakeStore{guard: openGuard(commandAgentThread)}
	bot := testBot(api, sprinterStore, scriptedRunner{reply: Reply{
		Text: "The model is rate limited. Try again in a minute.",
	}})

	interaction, data := command(commandAgentThread, general, leads)
	bot.handleCommand(interaction, data)

	if posts := api.only("followup"); len(posts) != 1 ||
		posts[0].content != "The model is rate limited. Try again in a minute." {
		t.Fatalf("the answer must still be posted as a follow-up: %+v", posts)
	}
	if threads := api.only("thread"); len(threads) != 0 {
		t.Fatalf("a thread was opened with no transcript: %+v", threads)
	}
	if len(sprinterStore.threads) != 0 {
		t.Fatalf("a thread row was written: %+v", sprinterStore.threads)
	}
	if len(sprinterStore.turns) != 0 {
		t.Fatalf("turns were invented: %+v", sprinterStore.turns)
	}
}

// With a transcript, the thread opens and stores exactly the runner's turns.
func TestATranscriptOpensTheThread(t *testing.T) {
	api := newFakeAPI()
	sprinterStore := &fakeStore{guard: openGuard(commandAgentThread)}
	turns := []model.Message{
		{Role: model.RoleUser, Text: "how is the platform?"},
		{Role: model.RoleAssistant, Text: "checking"},
		{Role: model.RoleAssistant, Text: "Everything is operational."},
	}
	bot := testBot(api, sprinterStore, scriptedRunner{reply: Reply{
		Text: "Everything is operational.", Messages: turns,
	}})

	interaction, data := command(commandAgentThread, general, leads)
	bot.handleCommand(interaction, data)

	if threads := api.only("thread"); len(threads) != 1 {
		t.Fatalf("calls = %v, want one thread", api.kinds())
	}
	if len(sprinterStore.threads) != 1 || sprinterStore.threads[0].ThreadID != "thread-1" {
		t.Fatalf("thread rows = %+v", sprinterStore.threads)
	}
	if len(sprinterStore.turns) != len(turns) {
		t.Fatalf("stored %d turns, want %d", len(sprinterStore.turns), len(turns))
	}
}

// `/agent` answers in the channel and never opens a thread, whatever the
// runner returned.
func TestShouldOpenThread(t *testing.T) {
	message := &discordgo.Message{ID: "message-1"}
	full := Reply{Text: "answer", Messages: []model.Message{{Role: model.RoleAssistant, Text: "answer"}}}
	empty := Reply{Text: "The model is rate limited. Try again in a minute."}

	cases := []struct {
		name    string
		subject string
		first   *discordgo.Message
		reply   Reply
		want    bool
	}{
		{"a thread command with a transcript", commandAgentThread, message, full, true},
		{"a thread command with no transcript", commandAgentThread, message, empty, false},
		{"a thread command whose answer never posted", commandAgentThread, nil, full, false},
		{"the plain command never opens a thread", commandAgent, message, full, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := shouldOpenThread(testCase.subject, testCase.first, testCase.reply); got != testCase.want {
				t.Fatalf("shouldOpenThread = %t, want %t", got, testCase.want)
			}
		})
	}
}

// Nothing the bot writes may ping a room. Sprinter repeats log lines and
// event titles other people wrote, so an "@everyone" in any of them would
// otherwise reach the whole server.
func TestEveryOutgoingMessageParsesNoMentions(t *testing.T) {
	none := noMentions()
	if none.Parse == nil {
		t.Fatal("Parse must be an empty slice, not nil: omitted, Discord parses every mention")
	}
	if len(none.Parse) != 0 || len(none.Roles) != 0 || len(none.Users) != 0 {
		t.Fatalf("allowed mentions = %+v, want nothing allowed", none)
	}
}

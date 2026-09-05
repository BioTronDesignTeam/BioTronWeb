package discord

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"
)

// api is everything the bot does to Discord after the gateway hands it an
// event. It is an interface the package owns, so the order of an
// acknowledgement and a guard read, whether a thread is opened at all, and
// what a message is allowed to ping are all tests rather than claims in a
// comment.
type api interface {
	// Acknowledge takes the interaction off Discord's three-second clock
	// with a deferred reply. Everything after it is a follow-up or an edit.
	Acknowledge(interaction *discordgo.Interaction) error
	// EditResponse replaces the deferred reply with content.
	EditResponse(interaction *discordgo.Interaction, content string) error
	// Followup posts another message under the same interaction.
	Followup(interaction *discordgo.Interaction, content string) (*discordgo.Message, error)
	// StartThread hangs a thread off one message and returns its id.
	StartThread(channelID, messageID, name string) (string, error)
	// Send posts a plain message to a channel or a thread.
	Send(channelID, content string) error
	// Typing shows the typing indicator once.
	Typing(channelID string)
	// ParentChannelID is the channel a thread hangs off, or "" when the id
	// is not a thread or cannot be read.
	ParentChannelID(channelID string) string
}

// noMentions is on every message the bot sends. Sprinter repeats text other
// people wrote — a log line, an event title, a model's summary of both — so
// an "@everyone" in any of it would otherwise ping the whole server. An
// empty Parse slice is not the same as no field: sent, it means "parse no
// mentions"; omitted, Discord parses every @ in the text.
func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}

// sessionAPI is api over a live gateway session. It is the only place in the
// bot that calls discordgo, and it gives every call the same deadline.
type sessionAPI struct {
	session *discordgo.Session
}

func (s sessionAPI) with() (discordgo.RequestOption, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
	return discordgo.WithContext(ctx), cancel
}

func (s sessionAPI) Acknowledge(interaction *discordgo.Interaction) error {
	option, cancel := s.with()
	defer cancel()
	return s.session.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}, option)
}

func (s sessionAPI) EditResponse(interaction *discordgo.Interaction, content string) error {
	option, cancel := s.with()
	defer cancel()
	_, err := s.session.InteractionResponseEdit(interaction, &discordgo.WebhookEdit{
		Content:         &content,
		AllowedMentions: noMentions(),
	}, option)
	return err
}

func (s sessionAPI) Followup(interaction *discordgo.Interaction, content string) (*discordgo.Message, error) {
	option, cancel := s.with()
	defer cancel()
	return s.session.FollowupMessageCreate(interaction, true, &discordgo.WebhookParams{
		Content:         content,
		AllowedMentions: noMentions(),
	}, option)
}

func (s sessionAPI) StartThread(channelID, messageID, name string) (string, error) {
	option, cancel := s.with()
	defer cancel()
	// 1440 minutes is a day: long enough for a conversation to resume the
	// next morning, short enough that the channel list does not fill with
	// threads.
	thread, err := s.session.MessageThreadStart(channelID, messageID, name, 1440, option)
	if err != nil {
		return "", err
	}
	return thread.ID, nil
}

func (s sessionAPI) Send(channelID, content string) error {
	option, cancel := s.with()
	defer cancel()
	_, err := s.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: noMentions(),
	}, option)
	return err
}

func (s sessionAPI) Typing(channelID string) {
	option, cancel := s.with()
	defer cancel()
	// A failed indicator is cosmetic. The answer still arrives.
	_ = s.session.ChannelTyping(channelID, option)
}

func (s sessionAPI) ParentChannelID(channelID string) string {
	// The gateway's own cache answers this without a round trip for every
	// thread the bot can see. The REST call is the fallback for one it has
	// not cached yet.
	if s.session.State != nil {
		if channel, err := s.session.State.Channel(channelID); err == nil && channel != nil {
			return channel.ParentID
		}
	}
	option, cancel := s.with()
	defer cancel()
	channel, err := s.session.Channel(channelID, option)
	if err != nil || channel == nil {
		return ""
	}
	return channel.ParentID
}

// typingInterval refreshes the indicator, which Discord clears after about
// ten seconds.
const typingInterval = 8 * time.Second

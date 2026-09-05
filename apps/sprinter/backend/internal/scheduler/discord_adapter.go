package scheduler

import (
	"github.com/bwmarrin/discordgo"
)

// DiscordSession adapts a live *discordgo.Session to DiscordAPI. It is the
// only file in this package that imports discordgo, so the rest of the
// scheduler is testable with no gateway session at all.
type DiscordSession struct {
	Session *discordgo.Session
}

func NewDiscordSession(session *discordgo.Session) DiscordSession {
	return DiscordSession{Session: session}
}

func (d DiscordSession) ChannelMessages(channelID string, limit int, beforeID, afterID string) ([]Message, error) {
	raw, err := d.Session.ChannelMessages(channelID, limit, beforeID, afterID, "")
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(raw))
	for _, m := range raw {
		message := Message{ID: m.ID, Timestamp: m.Timestamp}
		if m.Author != nil {
			message.AuthorID = m.Author.ID
			message.IsBot = m.Author.Bot
		}
		messages = append(messages, message)
	}
	return messages, nil
}

// mentions builds the allow-list Discord applies to one message. An empty
// Parse slice is not the same as no field at all: sent, it means "parse no
// mentions"; omitted, Discord parses every @ in the text. Only the ids in
// users may ping.
func mentions(users []string) *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{
		Parse: []discordgo.AllowedMentionType{},
		Users: users,
	}
}

func (d DiscordSession) SendMessage(channelID, content string, mentionUsers []string) (string, error) {
	message, err := d.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: mentions(mentionUsers),
	})
	if err != nil {
		return "", err
	}
	return message.ID, nil
}

func (d DiscordSession) EditMessage(channelID, messageID, content string) error {
	_, err := d.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:         channelID,
		ID:              messageID,
		Content:         &content,
		AllowedMentions: mentions(nil),
	})
	return err
}

func (d DiscordSession) DirectMessage(userID, content string) (string, error) {
	channel, err := d.Session.UserChannelCreate(userID)
	if err != nil {
		return "", err
	}
	// A DM already reaches the one person it is for, so it needs no ping.
	message, err := d.Session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: mentions(nil),
	})
	if err != nil {
		return "", err
	}
	return message.ID, nil
}

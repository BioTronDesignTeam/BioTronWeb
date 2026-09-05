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

func (d DiscordSession) SendMessage(channelID, content string) (string, error) {
	message, err := d.Session.ChannelMessageSend(channelID, content)
	if err != nil {
		return "", err
	}
	return message.ID, nil
}

func (d DiscordSession) EditMessage(channelID, messageID, content string) error {
	_, err := d.Session.ChannelMessageEdit(channelID, messageID, content)
	return err
}

func (d DiscordSession) DirectMessage(userID, content string) (string, error) {
	channel, err := d.Session.UserChannelCreate(userID)
	if err != nil {
		return "", err
	}
	message, err := d.Session.ChannelMessageSend(channel.ID, content)
	if err != nil {
		return "", err
	}
	return message.ID, nil
}

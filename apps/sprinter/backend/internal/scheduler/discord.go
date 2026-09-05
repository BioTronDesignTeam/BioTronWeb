package scheduler

import "time"

// Message is the part of a Discord message the scheduler reads back when it
// scans a channel's recent history for a lead's own announcement. It is
// deliberately not discordgo.Message: ID is kept only so a long history can
// be paged, and nothing else about a real Discord message matters here.
type Message struct {
	ID        string
	AuthorID  string
	IsBot     bool
	Timestamp time.Time
}

// DiscordAPI is the part of Discord the scheduler uses to read a channel's
// recent history and to post, edit, or DM. It is a small interface the
// scheduler package owns, so a test can fake Discord without a gateway
// session.
type DiscordAPI interface {
	// ChannelMessages returns up to limit messages, newest first, from
	// channelID. beforeID and afterID bound the window the same way
	// Discord's own API does; either may be empty.
	ChannelMessages(channelID string, limit int, beforeID, afterID string) ([]Message, error)
	// SendMessage posts content to channelID and returns the new message id.
	SendMessage(channelID, content string) (string, error)
	// EditMessage replaces messageID's content in channelID.
	EditMessage(channelID, messageID, content string) error
	// DirectMessage opens (or reuses) a DM with userID and sends content,
	// returning the new message id.
	DirectMessage(userID, content string) (string, error)
}

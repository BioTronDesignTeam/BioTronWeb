package scheduler

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

// Every message the scheduler posts or edits suppresses link previews, so a
// link in an event or a draft is never fetched by Discord on its own.
func TestEverySchedulerMessageSuppressesEmbeds(t *testing.T) {
	if send := sendPayload("hi https://example.com", []string{"lead"}); send.Flags&discordgo.MessageFlagsSuppressEmbeds == 0 {
		t.Fatalf("send flags = %v", send.Flags)
	}
	edit := editPayload("channel", "message", "Cancelled: https://example.com")
	if edit.Flags&discordgo.MessageFlagsSuppressEmbeds == 0 {
		t.Fatalf("edit flags = %v", edit.Flags)
	}
	if edit.AllowedMentions == nil || edit.AllowedMentions.Parse == nil {
		t.Fatal("an edit must still parse no mentions")
	}
}

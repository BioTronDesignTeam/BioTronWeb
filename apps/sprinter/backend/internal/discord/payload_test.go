package discord

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// Every message the bot sends, edits or defers suppresses link previews. A
// model talked into writing a link that carries data would otherwise have
// Discord fetch it, with no click, the moment the message posts.
func TestEveryBotMessageSuppressesEmbeds(t *testing.T) {
	suppressed := func(flags discordgo.MessageFlags) bool {
		return flags&discordgo.MessageFlagsSuppressEmbeds != 0
	}
	if ack := deferredResponse(); ack.Data == nil || !suppressed(ack.Data.Flags) {
		t.Fatalf("deferred response = %+v", ack)
	}
	if followup := followupParams("x"); !suppressed(followup.Flags) {
		t.Fatalf("follow-up flags = %v", followup.Flags)
	}
	if send := messageSend("x"); !suppressed(send.Flags) {
		t.Fatalf("send flags = %v", send.Flags)
	}
	// The edit is sent as raw JSON, so the wire form is what is checked.
	raw, err := json.Marshal(responseEdit("see https://example.com"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"flags":4`) || !strings.Contains(string(raw), `"allowed_mentions":{"parse":[]`) {
		t.Fatalf("edit body = %s", raw)
	}
}

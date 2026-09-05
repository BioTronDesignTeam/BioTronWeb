package discord

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

const (
	guild   = "100000000000000001"
	other   = "100000000000000002"
	general = "200000000000000001"
	offtop  = "200000000000000002"
	leads   = "300000000000000001"
	guests  = "300000000000000002"
	thread  = "500000000000000001"
)

func TestAllowedIn(t *testing.T) {
	anyChannel := store.Guard{Subject: store.SubjectAgent, GuildID: guild, RoleIDs: []string{leads}}
	oneChannel := store.Guard{
		Subject: store.SubjectAgent, GuildID: guild,
		RoleIDs: []string{leads}, ChannelIDs: []string{general},
	}

	cases := []struct {
		name  string
		guard store.Guard
		// channels is what the guard's channel list is matched against: the
		// channel the question came from, plus a thread's parent when there
		// is one.
		channels   []string
		guildID    string
		roles      []string
		hasMember  bool
		want       bool
		wantReason reason
	}{
		{
			name:  "a lead in the guild passes with no channel list",
			guard: anyChannel, guildID: guild, channels: []string{offtop},
			roles: []string{leads}, hasMember: true, want: true,
		},
		{
			name:  "a lead in a listed channel passes",
			guard: oneChannel, guildID: guild, channels: []string{general},
			roles: []string{leads}, hasMember: true, want: true,
		},
		{
			name:  "one matching role out of several is enough",
			guard: anyChannel, guildID: guild, channels: []string{general},
			roles: []string{guests, leads}, hasMember: true, want: true,
		},
		{
			// A thread's own id is never in a guard's channel list, so the
			// parent is offered alongside it. Matching only the thread id
			// would refuse every question asked inside a thread.
			name:  "a thread under a listed channel passes on its parent",
			guard: oneChannel, guildID: guild, channels: []string{thread, general},
			roles: []string{leads}, hasMember: true, want: true,
		},
		{
			name:  "a thread under an unlisted channel is refused",
			guard: oneChannel, guildID: guild, channels: []string{thread, offtop},
			roles: []string{leads}, hasMember: true, wantReason: reasonWrongChanel,
		},
		{
			name:  "a thread whose parent could not be read is refused on its own id",
			guard: oneChannel, guildID: guild, channels: []string{thread},
			roles: []string{leads}, hasMember: true, wantReason: reasonWrongChanel,
		},
		{
			name:  "a direct message has no guild",
			guard: anyChannel, guildID: "", channels: []string{general},
			roles: []string{leads}, hasMember: true, wantReason: reasonNoGuild,
		},
		{
			name:  "another guild is refused",
			guard: anyChannel, guildID: other, channels: []string{general},
			roles: []string{leads}, hasMember: true, wantReason: reasonWrongGuild,
		},
		{
			name:  "an unlisted channel is refused",
			guard: oneChannel, guildID: guild, channels: []string{offtop},
			roles: []string{leads}, hasMember: true, wantReason: reasonWrongChanel,
		},
		{
			name:  "no member means no roles to read",
			guard: anyChannel, guildID: guild, channels: []string{general},
			wantReason: reasonNoMember,
		},
		{
			name:  "a member without the role is refused",
			guard: anyChannel, guildID: guild, channels: []string{general},
			roles: []string{guests}, hasMember: true, wantReason: reasonWrongRole,
		},
		{
			name:    "a guard naming no role admits nobody",
			guard:   store.Guard{Subject: store.SubjectAgent, GuildID: guild},
			guildID: guild, channels: []string{general},
			roles: []string{leads}, hasMember: true, wantReason: reasonWrongRole,
		},
		{
			name:  "the channel is checked before the roles, so an outsider learns nothing",
			guard: oneChannel, guildID: guild, channels: []string{offtop},
			roles: []string{guests}, hasMember: true, wantReason: reasonWrongChanel,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ok, why := allowedIn(testCase.guard, testCase.guildID,
				testCase.channels, testCase.roles, testCase.hasMember)
			if ok != testCase.want || why != testCase.wantReason {
				t.Fatalf("allowedIn = (%t, %q), want (%t, %q)", ok, why, testCase.want, testCase.wantReason)
			}
			if !ok && refusals[why] == "" {
				t.Fatalf("no sentence for reason %q", why)
			}
		})
	}
}

func TestSplitMessageCutsOnLineBoundaries(t *testing.T) {
	long := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	parts := splitMessage(long, 40)
	if len(parts) != 2 || parts[0] != strings.Repeat("a", 30) || parts[1] != strings.Repeat("b", 30) {
		t.Fatalf("parts = %q", parts)
	}

	// A single line with nowhere to cut is cut at the limit.
	parts = splitMessage(strings.Repeat("c", 90), 40)
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	for _, part := range parts {
		if len(part) > 40 {
			t.Fatalf("part of %d characters exceeds the limit", len(part))
		}
	}

	if splitMessage("   \n ", 40) != nil {
		t.Fatal("an empty answer must produce no messages")
	}
	if parts := splitMessage("short", 40); len(parts) != 1 || parts[0] != "short" {
		t.Fatalf("parts = %q", parts)
	}
}

func TestThreadNameIsShortAndNeverEmpty(t *testing.T) {
	if name := threadName("  what is\nthe plan  "); name != "what is the plan" {
		t.Fatalf("name = %q", name)
	}
	if name := threadName(strings.Repeat("é", 200)); len([]rune(name)) != 80 {
		t.Fatalf("name = %d runes, want 80", len([]rune(name)))
	}
	if name := threadName("   "); name != "Agent thread" {
		t.Fatalf("name = %q", name)
	}
}

// The limit counts bytes and Discord counts characters. An answer with an
// accent or an emoji straddling the limit must not be cut through the middle
// of that character, or both messages carry half of one.
func TestSplitMessageNeverCutsThroughACharacter(t *testing.T) {
	// "é" is two bytes, so a 41-byte run of them has a character across
	// every odd byte index, including the limit.
	long := strings.Repeat("é", 41)
	for _, limit := range []int{40, 41, 39} {
		parts := splitMessage(long, limit)
		if len(parts) == 0 {
			t.Fatalf("limit %d produced nothing", limit)
		}
		rejoined := ""
		for i, part := range parts {
			if !utf8.ValidString(part) {
				t.Fatalf("limit %d, part %d is not valid UTF-8: %q", limit, i, part)
			}
			if len(part) > limit {
				t.Fatalf("limit %d, part %d is %d bytes", limit, i, len(part))
			}
			rejoined += part
		}
		if rejoined != long {
			t.Fatalf("limit %d lost or changed characters", limit)
		}
	}

	// A four-byte emoji next to a line break: the line boundary is the better
	// cut and must still leave both halves valid.
	mixed := strings.Repeat("🚀", 12) + "\n" + strings.Repeat("🚀", 12)
	for _, part := range splitMessage(mixed, 50) {
		if !utf8.ValidString(part) {
			t.Fatalf("part is not valid UTF-8: %q", part)
		}
	}

	// One character wider than the whole limit is taken whole rather than
	// looping forever on a cut that removes nothing.
	parts := splitMessage("🚀🚀", 2)
	if len(parts) != 2 || parts[0] != "🚀" || parts[1] != "🚀" {
		t.Fatalf("parts = %q", parts)
	}
}

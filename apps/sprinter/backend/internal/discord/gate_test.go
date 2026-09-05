package discord

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

const (
	guild   = "100000000000000001"
	other   = "100000000000000002"
	general = "200000000000000001"
	offtop  = "200000000000000002"
	leads   = "300000000000000001"
	guests  = "300000000000000002"
)

func interaction(guildID, channelID string, roles ...string) *discordgo.InteractionCreate {
	create := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		GuildID:   guildID,
		ChannelID: channelID,
	}}
	if roles != nil {
		create.Member = &discordgo.Member{Roles: roles, User: &discordgo.User{ID: "400000000000000001"}}
	}
	return create
}

func TestAllowed(t *testing.T) {
	anyChannel := store.Guard{Subject: store.SubjectAgent, GuildID: guild, RoleIDs: []string{leads}}
	oneChannel := store.Guard{
		Subject: store.SubjectAgent, GuildID: guild,
		RoleIDs: []string{leads}, ChannelIDs: []string{general},
	}

	cases := []struct {
		name        string
		guard       store.Guard
		interaction *discordgo.InteractionCreate
		want        bool
		wantReason  reason
	}{
		{
			name:        "a lead in the guild passes with no channel list",
			guard:       anyChannel,
			interaction: interaction(guild, offtop, leads),
			want:        true,
		},
		{
			name:        "a lead in a listed channel passes",
			guard:       oneChannel,
			interaction: interaction(guild, general, leads),
			want:        true,
		},
		{
			name:        "one matching role out of several is enough",
			guard:       anyChannel,
			interaction: interaction(guild, general, guests, leads),
			want:        true,
		},
		{
			name:        "a direct message has no guild",
			guard:       anyChannel,
			interaction: interaction("", general, leads),
			wantReason:  reasonNoGuild,
		},
		{
			name:        "another guild is refused",
			guard:       anyChannel,
			interaction: interaction(other, general, leads),
			wantReason:  reasonWrongGuild,
		},
		{
			name:        "an unlisted channel is refused",
			guard:       oneChannel,
			interaction: interaction(guild, offtop, leads),
			wantReason:  reasonWrongChanel,
		},
		{
			name:        "no member means no roles to read",
			guard:       anyChannel,
			interaction: interaction(guild, general),
			wantReason:  reasonNoMember,
		},
		{
			name:        "a member without the role is refused",
			guard:       anyChannel,
			interaction: interaction(guild, general, guests),
			wantReason:  reasonWrongRole,
		},
		{
			name:        "a guard naming no role admits nobody",
			guard:       store.Guard{Subject: store.SubjectAgent, GuildID: guild},
			interaction: interaction(guild, general, leads),
			wantReason:  reasonWrongRole,
		},
		{
			name:        "the channel is checked before the roles, so an outsider learns nothing",
			guard:       oneChannel,
			interaction: interaction(guild, offtop, guests),
			wantReason:  reasonWrongChanel,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ok, why := allowed(testCase.guard, testCase.interaction)
			if ok != testCase.want || why != testCase.wantReason {
				t.Fatalf("allowed = (%t, %q), want (%t, %q)", ok, why, testCase.want, testCase.wantReason)
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

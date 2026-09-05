package discord

import (
	"github.com/bwmarrin/discordgo"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/store"
)

// reason is why a command was refused. It goes into the log and picks the
// sentence the caller sees. The operator's question never goes into either.
type reason string

const (
	reasonNoGuard     reason = "no_guard"
	reasonNoGuild     reason = "no_guild"
	reasonWrongGuild  reason = "wrong_guild"
	reasonNoMember    reason = "no_member"
	reasonWrongRole   reason = "wrong_role"
	reasonWrongChanel reason = "wrong_channel"
	reasonUnavailable reason = "unavailable"
)

// refusals are what the person who ran the command reads. They say enough to
// act on and no more: naming the allowed roles or channels would leak the
// guard to someone the guard exists to keep out.
var refusals = map[reason]string{
	reasonNoGuard:     "This command is not configured yet. Ask an admin to set its guard.",
	reasonNoGuild:     "Run this in a server channel, not a direct message.",
	reasonWrongGuild:  "This command is not available in this server.",
	reasonNoMember:    "I could not read your roles. Try again in a server channel.",
	reasonWrongRole:   "You do not have a role that may use this command.",
	reasonWrongChanel: "This command is not allowed in this channel.",
	reasonUnavailable: "Sprinter cannot check permissions right now. Try again shortly.",
}

// allowed is the whole gate for one command, kept pure so every branch is a
// table row in the test rather than a Discord session.
//
// Three things must hold: the command ran in the guild the guard names, in a
// channel the guard lists or in any channel when it lists none, and by a
// member holding at least one role the guard names.
func allowed(guard store.Guard, interaction *discordgo.InteractionCreate) (bool, reason) {
	if interaction.GuildID == "" {
		return false, reasonNoGuild
	}
	if interaction.GuildID != guard.GuildID {
		return false, reasonWrongGuild
	}
	if len(guard.ChannelIDs) > 0 && !contains(guard.ChannelIDs, interaction.ChannelID) {
		return false, reasonWrongChanel
	}
	if interaction.Member == nil {
		return false, reasonNoMember
	}
	for _, role := range interaction.Member.Roles {
		if contains(guard.RoleIDs, role) {
			return true, ""
		}
	}
	return false, reasonWrongRole
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

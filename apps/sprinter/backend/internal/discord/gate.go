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
func allowed(guard store.Guard, interaction *discordgo.InteractionCreate) (bool, reason) {
	if interaction.Member == nil {
		return allowedIn(guard, interaction.GuildID, interaction.ChannelID, nil, false)
	}
	return allowedIn(guard, interaction.GuildID, interaction.ChannelID, interaction.Member.Roles, true)
}

// allowedIn is the gate itself, over ids rather than over a Discord type. A
// thread follow-up is a message, not an interaction, and it has to pass the
// same gate; sharing this function is what stops the two paths from drifting.
//
// Three things must hold: the question came from the guild the guard names,
// from a channel the guard lists or from any channel when it lists none, and
// from a member holding at least one role the guard names.
//
// For a thread, channelID is the thread's parent channel. A thread has an id
// of its own that no guard could ever list, so checking the thread's id would
// refuse every follow-up in a guard that names channels.
func allowedIn(guard store.Guard, guildID, channelID string, roles []string, hasMember bool) (bool, reason) {
	if guildID == "" {
		return false, reasonNoGuild
	}
	if guildID != guard.GuildID {
		return false, reasonWrongGuild
	}
	if len(guard.ChannelIDs) > 0 && !contains(guard.ChannelIDs, channelID) {
		return false, reasonWrongChanel
	}
	if !hasMember {
		return false, reasonNoMember
	}
	for _, role := range roles {
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

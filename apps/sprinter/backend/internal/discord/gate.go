package discord

import (
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

// allowedIn is the gate itself, over ids rather than over a Discord type. A
// thread follow-up is a message, not an interaction, and it has to pass the
// same gate; sharing this function is what stops the two paths from drifting.
//
// Three things must hold: the question came from the guild the guard names,
// from a channel the guard lists or from any channel when it lists none, and
// from a member holding at least one role the guard names.
//
// channelIDs is every id the guard's channel list may match for this
// question: the channel it came from, and, when that is a thread, the
// thread's parent. A guard names channels, and Discord gives a thread an id
// of its own that no guard could list, so matching the thread id alone would
// refuse every question asked inside one.
func allowedIn(guard store.Guard, guildID string, channelIDs, roles []string, hasMember bool) (bool, reason) {
	if guildID == "" {
		return false, reasonNoGuild
	}
	if guildID != guard.GuildID {
		return false, reasonWrongGuild
	}
	if len(guard.ChannelIDs) > 0 && !containsAny(guard.ChannelIDs, channelIDs) {
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

// containsAny reports whether any of wanted is in values.
func containsAny(values, wanted []string) bool {
	for _, candidate := range wanted {
		if contains(values, candidate) {
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

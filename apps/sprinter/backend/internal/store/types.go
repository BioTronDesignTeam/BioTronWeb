package store

import (
	"encoding/json"
	"time"
)

// The guard subjects Sprinter knows. A command whose subject has no guard row
// is refused, so adding a command without a guard fails closed.
const (
	SubjectAgent       = "agent"
	SubjectAgentThread = "agent-thread"
	SubjectAnnounce    = "announce"
	SubjectNudge       = "nudge"
)

// The automation kinds and delivery modes, spelled as the enum values the
// migration created.
const (
	KindAnnounce = "ANNOUNCE"
	KindNudge    = "NUDGE"

	DeliverDM      = "DM"
	DeliverChannel = "CHANNEL"
)

// Guard is who may use one bot feature. RoleIDs lists the Discord roles that
// may; an empty ChannelIDs means any channel in the guild.
type Guard struct {
	Subject    string    `json:"subject"`
	GuildID    string    `json:"guild_id"`
	RoleIDs    []string  `json:"role_ids"`
	ChannelIDs []string  `json:"channel_ids"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Automation is one scheduled job: an ANNOUNCE that posts Calendar events to a
// channel, or a NUDGE that chases a lead about them. The two kinds share a
// table because they share a scope, a channel, and an enabled flag; the fields
// each ignores keep their defaults.
type Automation struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	ScopeID       string    `json:"scope_id"`
	ChannelID     string    `json:"channel_id"`
	LeadUserID    *string   `json:"lead_user_id"`
	LeadHours     int       `json:"lead_hours"`
	LookbackHours int       `json:"lookback_hours"`
	AnyAuthor     bool      `json:"any_author"`
	PostHour      *int      `json:"post_hour"`
	Deliver       string    `json:"deliver"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Thread is one `/agent-thread` conversation, keyed by the Discord thread the
// bot opened. TurnCount is how many questions it has answered.
type Thread struct {
	ThreadID     string    `json:"thread_id"`
	GuildID      string    `json:"guild_id"`
	ChannelID    string    `json:"channel_id"`
	OpenerID     string    `json:"opener_id"`
	Model        string    `json:"model"`
	TurnCount    int       `json:"turn_count"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// Message is one turn of a thread transcript. Content is the JSON encoding of
// model.Message, so the agent loop can replay a thread without a schema change
// each time a vendor grows a field.
type Message struct {
	ID        int64           `json:"id"`
	ThreadID  string          `json:"thread_id"`
	Sequence  int             `json:"sequence"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	CreatedAt time.Time       `json:"created_at"`
}

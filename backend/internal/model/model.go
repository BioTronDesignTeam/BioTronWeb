package model

import (
	"encoding/json"
	"time"
)

const (
	ScopeTeam     = "TEAM"
	ScopeProject  = "PROJECT"
	ScopeSubteam  = "SUBTEAM"
	ScopeActive   = "ACTIVE"
	ScopeArchived = "ARCHIVED"

	EventDraft     = "DRAFT"
	EventPublished = "PUBLISHED"
	EventCancelled = "CANCELLED"

	OverrideModified  = "MODIFIED"
	OverrideCancelled = "CANCELLED"
)

type Scope struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Path and SlugPath qualify a scope by its ancestors, so that the two
	// subteams both named "Software" read "Exo · Software" and
	// "Enable · Software" wherever they appear outside the scope tree. The
	// team root is left out: it is the same for every scope and adds nothing.
	Path       string     `json:"path"`
	SlugPath   string     `json:"slug_path"`
	Status     string     `json:"status"`
	ParentID   *string    `json:"parent_id,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ChildCount int        `json:"child_count"`
	EventCount int        `json:"event_count"`
}

type EventSeries struct {
	ID              string          `json:"id"`
	UID             string          `json:"uid"`
	ScopeID         string          `json:"scope_id"`
	ScopeName       string          `json:"scope_name,omitempty"`
	ScopePath       string          `json:"scope_path,omitempty"`
	ScopeKind       string          `json:"scope_kind,omitempty"`
	State           string          `json:"state"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	Location        string          `json:"location"`
	URL             string          `json:"url"`
	StartsAtLocal   time.Time       `json:"starts_at_local"`
	EndsAtLocal     time.Time       `json:"ends_at_local"`
	Timezone        string          `json:"timezone"`
	AllDay          bool            `json:"all_day"`
	RecurrenceUntil *time.Time      `json:"recurrence_until,omitempty"`
	Sequence        int             `json:"sequence"`
	PublishedAt     *time.Time      `json:"published_at,omitempty"`
	CancelledAt     *time.Time      `json:"cancelled_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Overrides       []EventOverride `json:"overrides,omitempty"`
}

type EventOverride struct {
	ID                string          `json:"id"`
	SeriesID          string          `json:"series_id"`
	RecurrenceIDLocal time.Time       `json:"recurrence_id_local"`
	State             string          `json:"state"`
	Patch             json.RawMessage `json:"patch"`
	Sequence          int             `json:"sequence"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type Occurrence struct {
	SeriesID         string    `json:"series_id"`
	UID              string    `json:"uid"`
	ScopeID          string    `json:"scope_id"`
	ScopeName        string    `json:"scope_name"`
	ScopePath        string    `json:"scope_path"`
	ScopeKind        string    `json:"scope_kind"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Location         string    `json:"location"`
	URL              string    `json:"url"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	AllDay           bool      `json:"all_day"`
	Timezone         string    `json:"timezone"`
	RecurrenceID     string    `json:"recurrence_id_local"`
	Recurring        bool      `json:"recurring"`
	Modified         bool      `json:"modified"`
	SeriesSequence   int       `json:"series_sequence"`
	OverrideSequence int       `json:"override_sequence,omitempty"`
}

type Operator struct {
	GitHubID    int64  `json:"github_id"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	IsSuperuser bool   `json:"is_superuser"`
	IsManager   bool   `json:"is_manager"`
	IsStaff     bool   `json:"is_staff"`
}

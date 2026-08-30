package model

import (
	"encoding/json"
	"time"
)

type LogLevel string

const (
	LevelDebug   LogLevel = "debug"
	LevelInfo    LogLevel = "info"
	LevelWarning LogLevel = "warning"
	LevelError   LogLevel = "error"
)

func (l LogLevel) Valid() bool {
	switch l {
	case LevelDebug, LevelInfo, LevelWarning, LevelError:
		return true
	default:
		return false
	}
}

type Log struct {
	ID        string          `json:"id"`
	Service   string          `json:"service"`
	Level     LogLevel        `json:"level"`
	Message   string          `json:"message"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type NewLog struct {
	Service string          `json:"service"`
	Level   LogLevel        `json:"level"`
	Message string          `json:"message"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Health struct {
	Service   string    `json:"service"`
	OK        bool      `json:"ok"`
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type HistoryQuery struct {
	Services []string
	Levels   []LogLevel
	From     *time.Time
	To       *time.Time
	Search   string
	Cursor   string
	Limit    int
}

type LogPage struct {
	Logs       []Log  `json:"logs"`
	NextCursor string `json:"next_cursor,omitempty"`
}

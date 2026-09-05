// Package model is the one seam between the agent loop and a vendor's SDK.
//
// The loop, the tools, the scheduler, and the tests depend on the Model
// interface and the plain types here. A vendor lives in a subpackage
// (model/gemini today) and nothing else imports it, so changing vendors is
// one file in main.go.
package model

import (
	"context"
	"encoding/json"
)

// Role names who produced a message. Tool results are user-side content in
// every vendor's wire format, so they use RoleUser with ToolResults set.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn of a transcript. An assistant turn carries Text and
// zero or more ToolCalls. A user turn carries Text, or the ToolResults that
// answer the assistant's calls, never both.
type Message struct {
	Role        Role         `json:"role"`
	Text        string       `json:"text,omitempty"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
}

// ToolCall is a request from the model to run one tool with JSON arguments.
// ID ties the later ToolResult back to it; a vendor without call ids gets a
// synthetic one from the adapter.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// ToolResult is what a tool returned, already capped to a size the loop is
// willing to put in front of the model.
type ToolResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// ToolSpec describes a tool to the model. Schema is a JSON Schema object for
// the arguments.
type ToolSpec struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Response is one model turn. Text and ToolCalls may both be set; the loop
// runs the calls when there are any and treats Text alone as the answer.
type Response struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
}

// Model generates one assistant turn from a system prompt, the transcript so
// far, and the tools it may call. Implementations must be safe for
// concurrent use.
type Model interface {
	// Name is the vendor and model id, for logs and the agent_threads row.
	Name() string
	Generate(ctx context.Context, system string, history []Message, tools []ToolSpec) (Response, error)
}

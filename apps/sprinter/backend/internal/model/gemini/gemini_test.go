package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
	"github.com/BioTronDesignTeam/Sprinter/backend/internal/tools"
)

// These tests never reach the network. Everything that can be wrong without a
// key is a mapping: the schema, the roles, the missing call ids, the status
// code that means "wait".

func TestConvertSchema(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "object",
		"description": "arguments",
		"properties": {
			"service": {"type": "string", "description": "which service"},
			"level": {"type": "string", "enum": ["debug", "info"]},
			"limit": {"type": "integer", "description": "how many"},
			"ratio": {"type": "number"},
			"verbose": {"type": "boolean"},
			"names": {"type": "array", "items": {"type": "string"}}
		},
		"required": ["service"]
	}`)
	schema, err := convertSchema(raw)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if schema.Type != genai.TypeObject || schema.Description != "arguments" {
		t.Fatalf("root = %+v", schema)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "service" {
		t.Fatalf("required = %v", schema.Required)
	}
	// The required argument is listed first; the rest are alphabetical, so a
	// declaration does not change shape between runs.
	want := []string{"service", "level", "limit", "names", "ratio", "verbose"}
	if len(schema.PropertyOrdering) != len(want) {
		t.Fatalf("ordering = %v", schema.PropertyOrdering)
	}
	for i, name := range want {
		if schema.PropertyOrdering[i] != name {
			t.Fatalf("ordering = %v, want %v", schema.PropertyOrdering, want)
		}
	}
	kinds := map[string]genai.Type{
		"service": genai.TypeString, "level": genai.TypeString,
		"limit": genai.TypeInteger, "ratio": genai.TypeNumber,
		"verbose": genai.TypeBoolean, "names": genai.TypeArray,
	}
	for name, kind := range kinds {
		if got := schema.Properties[name]; got == nil || got.Type != kind {
			t.Fatalf("property %q = %+v, want type %s", name, got, kind)
		}
	}
	if level := schema.Properties["level"]; level.Format != "enum" || len(level.Enum) != 2 {
		t.Fatalf("enum property = %+v", level)
	}
	if items := schema.Properties["names"].Items; items == nil || items.Type != genai.TypeString {
		t.Fatalf("array items = %+v", items)
	}
	if schema.Properties["service"].Description != "which service" {
		t.Fatal("a property description must survive the conversion")
	}
}

func TestConvertSchemaRejectsWhatItCannotSend(t *testing.T) {
	cases := map[string]string{
		"an unknown type":        `{"type": "null"}`,
		"an array with no items": `{"type": "array"}`,
		"properties on a string": `{"type": "string", "properties": {"a": {"type": "string"}}}`,
		"a bad nested type":      `{"type": "object", "properties": {"a": {"type": "date"}}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := convertSchema(json.RawMessage(raw)); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	if schema, err := convertSchema(nil); err != nil || schema != nil {
		t.Fatalf("an absent schema must convert to nothing: %+v, %v", schema, err)
	}
}

func TestToContentsMapsRolesAndToolTurns(t *testing.T) {
	history := []model.Message{
		{Role: model.RoleUser, Text: "which services logged errors?"},
		{Role: model.RoleAssistant, Text: "checking", ToolCalls: []model.ToolCall{{
			ID: syntheticID("recent_logs", 1, 0), Name: "recent_logs",
			Args: json.RawMessage(`{"level":"error","limit":5}`),
		}}},
		{Role: model.RoleUser, ToolResults: []model.ToolResult{{
			ID: syntheticID("recent_logs", 1, 0), Name: "recent_logs", Content: "two rows",
		}}},
		{Role: model.RoleAssistant, Text: "two services did"},
	}
	contents, err := toContents(history)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(contents) != 4 {
		t.Fatalf("contents = %d, want 4", len(contents))
	}
	roles := []string{genai.RoleUser, genai.RoleModel, genai.RoleUser, genai.RoleModel}
	for i, role := range roles {
		if contents[i].Role != role {
			t.Fatalf("content %d role = %q, want %q", i, contents[i].Role, role)
		}
	}

	call := contents[1]
	if len(call.Parts) != 2 || call.Parts[0].Text != "checking" {
		t.Fatalf("assistant parts = %+v", call.Parts)
	}
	if call.Parts[1].FunctionCall == nil || call.Parts[1].FunctionCall.Name != "recent_logs" {
		t.Fatalf("function call part = %+v", call.Parts[1])
	}
	if call.Parts[1].FunctionCall.Args["level"] != "error" {
		t.Fatalf("args = %+v", call.Parts[1].FunctionCall.Args)
	}
	// A synthesised id is this adapter's bookkeeping. Sending it back would
	// claim Gemini issued an id it never issued.
	if call.Parts[1].FunctionCall.ID != "" {
		t.Fatalf("a synthetic id must not travel back: %q", call.Parts[1].FunctionCall.ID)
	}

	answer := contents[2].Parts[0].FunctionResponse
	if answer == nil || answer.Name != "recent_logs" || answer.Response["output"] != "two rows" {
		t.Fatalf("function response part = %+v", answer)
	}
	if answer.ID != "" {
		t.Fatalf("a synthetic id must not travel back: %q", answer.ID)
	}
}

func TestToContentsMarksAnErroredToolAndKeepsARealID(t *testing.T) {
	contents, err := toContents([]model.Message{
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "vendor-1", Name: "recent_logs"}}},
		{Role: model.RoleUser, ToolResults: []model.ToolResult{{
			ID: "vendor-1", Name: "recent_logs", Content: "limit must be 1 to 50", IsError: true,
		}}},
		{Role: model.RoleAssistant},
	})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// The empty assistant turn carries nothing Gemini would accept, so it is
	// dropped rather than sent as a part-less content.
	if len(contents) != 2 {
		t.Fatalf("contents = %d, want 2", len(contents))
	}
	if got := contents[0].Parts[0].FunctionCall.ID; got != "vendor-1" {
		t.Fatalf("call id = %q, want vendor-1", got)
	}
	response := contents[1].Parts[0].FunctionResponse
	if response.ID != "vendor-1" {
		t.Fatalf("response id = %q, want vendor-1", response.ID)
	}
	if _, ok := response.Response["error"]; !ok {
		t.Fatalf("an errored result must be sent as an error: %+v", response.Response)
	}
}

func TestFromResponseSynthesisesCallIDsAndCountsTokens(t *testing.T) {
	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{
			{Text: "thinking out loud", Thought: true},
			{Text: "  looking now  "},
			{FunctionCall: &genai.FunctionCall{Name: "recent_logs", Args: map[string]any{"limit": float64(5)}}},
			{FunctionCall: &genai.FunctionCall{Name: "recent_logs", Args: map[string]any{"limit": float64(9)}}},
		}}}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 120, CandidatesTokenCount: 30, ThoughtsTokenCount: 7,
		},
	}
	got := fromResponse(response)
	if got.Text != "looking now" {
		t.Fatalf("text = %q", got.Text)
	}
	if got.InputTokens != 120 || got.OutputTokens != 37 {
		t.Fatalf("tokens = %d in, %d out", got.InputTokens, got.OutputTokens)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("calls = %d, want 2", len(got.ToolCalls))
	}
	// Two calls to the same tool in one turn must not share an id, or the loop
	// cannot say which result answers which call.
	if got.ToolCalls[0].ID == got.ToolCalls[1].ID {
		t.Fatalf("both calls have id %q", got.ToolCalls[0].ID)
	}
	if string(got.ToolCalls[0].Args) != `{"limit":5}` {
		t.Fatalf("args = %s", got.ToolCalls[0].Args)
	}

	if empty := fromResponse(&genai.GenerateContentResponse{}); empty.Text != "" || empty.ToolCalls != nil {
		t.Fatalf("an empty response must map to an empty turn: %+v", empty)
	}
}

func TestMapErrorNamesTheRateLimit(t *testing.T) {
	limited := mapError(genai.APIError{
		Code: http.StatusTooManyRequests, Status: "RESOURCE_EXHAUSTED", Message: "quota",
	})
	if !errors.Is(limited, model.ErrRateLimited) {
		t.Fatalf("429 must map to ErrRateLimited, got %v", limited)
	}
	other := mapError(genai.APIError{
		Code: http.StatusInternalServerError, Status: "INTERNAL", Message: "boom",
	})
	if errors.Is(other, model.ErrRateLimited) {
		t.Fatalf("500 must not map to ErrRateLimited, got %v", other)
	}
	if mapError(errors.New("dial failed")) == nil {
		t.Fatal("a transport error must stay an error")
	}
}

// Gemini echoes the prompt back inside Message and Details. This error is
// written to the process log, so only the status code and the status name may
// come out of it.
func TestMapErrorNeverRepeatsTheResponseBody(t *testing.T) {
	secret := "the operator asked about token sk-live-4242"
	cases := map[string]genai.APIError{
		"rate limited": {
			Code: http.StatusTooManyRequests, Status: "RESOURCE_EXHAUSTED", Message: secret,
			Details: []map[string]any{{"reason": secret}},
		},
		"any other status": {
			Code: http.StatusBadRequest, Status: "INVALID_ARGUMENT", Message: secret,
			Details: []map[string]any{{"reason": secret}},
		},
	}
	for name, apiErr := range cases {
		t.Run(name, func(t *testing.T) {
			got := mapError(apiErr).Error()
			if strings.Contains(got, secret) || strings.Contains(got, "sk-live-4242") {
				t.Fatalf("the response body reached the error: %q", got)
			}
			want := fmt.Sprintf("gemini: status %d %s", apiErr.Code, apiErr.Status)
			if !strings.Contains(got, want) {
				t.Fatalf("error = %q, want it to contain %q", got, want)
			}
		})
	}
}

// A candidate can arrive as a null in the JSON array. Reading its content
// without checking the pointer panics the whole answer.
func TestFromResponseSurvivesANilCandidate(t *testing.T) {
	got := fromResponse(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{nil},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 11, CandidatesTokenCount: 0,
		},
	})
	if got.Text != "" || got.ToolCalls != nil {
		t.Fatalf("a nil candidate must map to an empty turn: %+v", got)
	}
	// The usage is still real and still billed, so it is still counted.
	if got.InputTokens != 11 {
		t.Fatalf("input tokens = %d, want 11", got.InputTokens)
	}
}

// The loop pairs a result to its call by id. Two turns that each start with
// the same tool used to invent the same id, so a replayed transcript could
// not say which result answered which call.
func TestSyntheticCallIDsAreUniqueAcrossResponses(t *testing.T) {
	one := func() *genai.GenerateContentResponse {
		return &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{Name: "recent_logs"}},
			}}}},
		}
	}
	first := fromResponse(one())
	second := fromResponse(one())
	if first.ToolCalls[0].ID == second.ToolCalls[0].ID {
		t.Fatalf("two turns share the call id %q", first.ToolCalls[0].ID)
	}
	// It must still be recognisable as this adapter's own invention, or it
	// would travel back to Gemini as an id the model never issued.
	if !strings.HasPrefix(second.ToolCalls[0].ID, syntheticPrefix) {
		t.Fatalf("id = %q, want the synthetic prefix", second.ToolCalls[0].ID)
	}
}

func TestNewRefusesAnEmptyKeyOrModel(t *testing.T) {
	if _, err := New(t.Context(), "", "gemini-2.5-flash"); err == nil {
		t.Fatal("an empty key must be refused")
	}
	if _, err := New(t.Context(), "key", " "); err == nil {
		t.Fatal("an empty model id must be refused")
	}
}

func TestNameCarriesTheModelID(t *testing.T) {
	m := &Model{id: "gemini-2.5-flash"}
	if m.Name() != "gemini/gemini-2.5-flash" {
		t.Fatalf("name = %q", m.Name())
	}
}

// TestEveryRealToolDeclarationConverts is the check that matters most: the
// schemas the tools actually ship must be sendable. A schema keyword this
// adapter cannot convert would otherwise be found by a person in Discord.
func TestEveryRealToolDeclarationConverts(t *testing.T) {
	var specs []model.ToolSpec
	for _, tool := range tools.All(nil, "http://logger", "http://calendar") {
		specs = append(specs, tool.Spec())
	}
	// The SQL tools need a reader to be built, so their specs are taken from
	// the types directly rather than from All.
	specs = append(specs,
		tools.RecentLogs{}.Spec(), tools.LogHistory{}.Spec(),
		tools.HealthHistory{}.Spec(), tools.WhoHasAccess{}.Spec())

	declarations, err := toDeclarations(specs)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	for i, declaration := range declarations {
		if declaration.Name == "" || declaration.Description == "" {
			t.Fatalf("declaration %d is missing a name or a description", i)
		}
		if declaration.Parameters == nil || declaration.Parameters.Type != genai.TypeObject {
			t.Fatalf("tool %q takes %+v, want an object", declaration.Name, declaration.Parameters)
		}
	}
}

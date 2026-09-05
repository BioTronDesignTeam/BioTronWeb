// Package gemini is the Gemini adapter behind model.Model.
//
// Nothing outside main.go imports it. Everything Gemini-shaped stops here:
// the upper-case schema types, the "model" role, the missing call ids, and
// the HTTP status that means "come back later".
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"google.golang.org/genai"

	"github.com/BioTronDesignTeam/Sprinter/backend/internal/model"
)

const (
	// maxOutputTokens is about four Discord messages. An answer longer than
	// that is a wall of text nobody reads, and the loop splits it anyway.
	maxOutputTokens = 2048
	// temperature is low because every answer is grounded in tool output. The
	// job is to report what the rows say, not to write two different reports
	// from the same rows.
	temperature = 0.2
)

// Model answers with one Gemini model. The genai client is safe for concurrent
// use, so one Model serves every thread.
type Model struct {
	client *genai.Client
	id     string
}

// New builds the adapter. It does not reach the network: a bad key is found on
// the first question, not at start, so a key rotation cannot stop the bot from
// booting.
func New(ctx context.Context, apiKey, modelID string) (*Model, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("gemini: API key is empty")
	}
	if strings.TrimSpace(modelID) == "" {
		return nil, errors.New("gemini: model id is empty")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: new client: %w", err)
	}
	return &Model{client: client, id: modelID}, nil
}

func (m *Model) Name() string { return "gemini/" + m.id }

func (m *Model) Generate(ctx context.Context, system string, history []model.Message, tools []model.ToolSpec) (model.Response, error) {
	contents, err := toContents(history)
	if err != nil {
		return model.Response{}, err
	}
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: maxOutputTokens,
		Temperature:     genai.Ptr[float32](temperature),
	}
	if system != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: system}}}
	}
	if len(tools) > 0 {
		declarations, err := toDeclarations(tools)
		if err != nil {
			return model.Response{}, err
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: declarations}}
	}

	response, err := m.client.Models.GenerateContent(ctx, m.id, contents, config)
	if err != nil {
		return model.Response{}, mapError(err)
	}
	return fromResponse(response), nil
}

// mapError turns Gemini's quota refusal into the sentinel the loop knows, and
// leaves every other failure alone. 429 is the only status the operator can do
// something about by waiting.
//
// An API error is reported as its status code and status name and nothing
// else. Message and Details are the server's own body: Gemini quotes the
// prompt back in them, and this error reaches the process log, so repeating
// them would write the question — and anything a tool put in front of the
// model — into the logs.
func mapError(err error) error {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code == http.StatusTooManyRequests {
			return fmt.Errorf("%w: gemini: status %d %s", model.ErrRateLimited, apiErr.Code, apiErr.Status)
		}
		return fmt.Errorf("gemini: status %d %s", apiErr.Code, apiErr.Status)
	}
	return fmt.Errorf("gemini: generate: %w", err)
}

func toDeclarations(tools []model.ToolSpec) ([]*genai.FunctionDeclaration, error) {
	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, spec := range tools {
		parameters, err := convertSchema(spec.Schema)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", spec.Name, err)
		}
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  parameters,
		})
	}
	return declarations, nil
}

// syntheticPrefix marks a call id this adapter invented. The Gemini API does
// not give function calls an id, but the loop pairs a result to its call by
// one, so the adapter makes one up and then has to recognise its own work: an
// invented id must not travel back to Gemini as if the model had issued it.
const syntheticPrefix = "gemini-call-"

// responseSeq numbers each response this process maps. Without it, two turns
// that both call recent_logs first would each invent
// "gemini-call-recent_logs-0", and a transcript replayed later could not say
// which result answered which call.
var responseSeq atomic.Uint64

func syntheticID(name string, response uint64, index int) string {
	return syntheticPrefix + name + "-" +
		strconv.FormatUint(response, 10) + "-" + strconv.Itoa(index)
}

func toContents(history []model.Message) ([]*genai.Content, error) {
	contents := make([]*genai.Content, 0, len(history))
	for _, message := range history {
		content, err := toContent(message)
		if err != nil {
			return nil, err
		}
		if content != nil {
			contents = append(contents, content)
		}
	}
	return contents, nil
}

func toContent(message model.Message) (*genai.Content, error) {
	var parts []*genai.Part
	if message.Text != "" {
		parts = append(parts, &genai.Part{Text: message.Text})
	}
	for _, call := range message.ToolCalls {
		args, err := toArgs(call.Args)
		if err != nil {
			return nil, fmt.Errorf("tool call %q: %w", call.Name, err)
		}
		part := &genai.Part{FunctionCall: &genai.FunctionCall{Name: call.Name, Args: args}}
		if !strings.HasPrefix(call.ID, syntheticPrefix) {
			part.FunctionCall.ID = call.ID
		}
		parts = append(parts, part)
	}
	for _, result := range message.ToolResults {
		// Gemini reads "output" as the value and "error" as a failure the model
		// should account for rather than repeat as an answer.
		key := "output"
		if result.IsError {
			key = "error"
		}
		part := &genai.Part{FunctionResponse: &genai.FunctionResponse{
			Name:     result.Name,
			Response: map[string]any{key: result.Content},
		}}
		if !strings.HasPrefix(result.ID, syntheticPrefix) {
			part.FunctionResponse.ID = result.ID
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		// An empty turn is not an error worth failing a question over, but it
		// is not something Gemini accepts either, so it is dropped.
		return nil, nil
	}
	role := genai.RoleUser
	if message.Role == model.RoleAssistant {
		role = genai.RoleModel
	}
	return &genai.Content{Role: role, Parts: parts}, nil
}

// toArgs decodes a tool call's arguments back into the map genai wants. Empty
// arguments are a legal call to a tool whose properties are all optional.
func toArgs(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	return args, nil
}

func fromResponse(response *genai.GenerateContentResponse) model.Response {
	var out model.Response
	if response == nil {
		return out
	}
	if usage := response.UsageMetadata; usage != nil {
		out.InputTokens = int(usage.PromptTokenCount)
		// Thoughts are billed as output and are invisible to the loop, so
		// leaving them out would under-report what the question cost.
		out.OutputTokens = int(usage.CandidatesTokenCount + usage.ThoughtsTokenCount)
	}
	// A candidate can come back as a null in the JSON array, so the pointer
	// is checked before its content is read.
	if len(response.Candidates) == 0 || response.Candidates[0] == nil ||
		response.Candidates[0].Content == nil {
		return out
	}

	sequence := responseSeq.Add(1)
	var text strings.Builder
	index := 0
	for _, part := range response.Candidates[0].Content.Parts {
		if part == nil || part.Thought {
			continue
		}
		if part.Text != "" {
			text.WriteString(part.Text)
		}
		if part.FunctionCall == nil {
			continue
		}
		args, err := json.Marshal(part.FunctionCall.Args)
		if err != nil {
			// Args came off the wire as JSON, so this cannot fail in practice.
			// An empty object still names the tool, and the tool's own
			// validation then reports what is missing.
			args = []byte("{}")
		}
		id := part.FunctionCall.ID
		if id == "" {
			id = syntheticID(part.FunctionCall.Name, sequence, index)
		}
		out.ToolCalls = append(out.ToolCalls, model.ToolCall{
			ID:   id,
			Name: part.FunctionCall.Name,
			Args: args,
		})
		index++
	}
	out.Text = strings.TrimSpace(text.String())
	return out
}

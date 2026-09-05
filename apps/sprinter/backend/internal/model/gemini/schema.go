package gemini

import (
	"encoding/json"
	"fmt"
	"slices"

	"google.golang.org/genai"
)

// jsonSchema is the subset of JSON Schema the tools write. Nothing here parses
// a schema a stranger wrote: every schema is a constant in internal/tools, so
// an unsupported keyword is a bug caught at start rather than a case to
// tolerate at runtime.
type jsonSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description"`
	Enum        []string              `json:"enum"`
	Properties  map[string]jsonSchema `json:"properties"`
	Required    []string              `json:"required"`
	Items       *jsonSchema           `json:"items"`
}

// convertSchema turns a tool's JSON Schema into the Schema genai sends. The
// two formats differ in one visible way — genai names types in upper case —
// and in one invisible way: Go maps have no order, so property order is fixed
// here from the schema's own required list plus the remaining names sorted, to
// keep a declaration byte-identical between runs.
func convertSchema(raw json.RawMessage) (*genai.Schema, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var parsed jsonSchema
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse tool schema: %w", err)
	}
	return convertNode(parsed, "")
}

func convertNode(node jsonSchema, path string) (*genai.Schema, error) {
	where := path
	if where == "" {
		where = "(root)"
	}
	kind, err := convertType(node.Type)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", where, err)
	}
	schema := &genai.Schema{
		Type:        kind,
		Description: node.Description,
		Enum:        node.Enum,
	}
	if len(node.Enum) > 0 {
		// genai only treats a list as a closed set when the format says so.
		schema.Format = "enum"
	}
	if kind == genai.TypeArray {
		if node.Items == nil {
			return nil, fmt.Errorf("%s: an array schema needs items", where)
		}
		items, err := convertNode(*node.Items, path+"[]")
		if err != nil {
			return nil, err
		}
		schema.Items = items
	}
	if len(node.Properties) > 0 {
		if kind != genai.TypeObject {
			return nil, fmt.Errorf("%s: only an object schema may have properties", where)
		}
		schema.Properties = map[string]*genai.Schema{}
		for _, name := range orderProperties(node) {
			child, err := convertNode(node.Properties[name], join(path, name))
			if err != nil {
				return nil, err
			}
			schema.Properties[name] = child
		}
		schema.PropertyOrdering = orderProperties(node)
		schema.Required = node.Required
	}
	return schema, nil
}

func convertType(name string) (genai.Type, error) {
	switch name {
	case "object":
		return genai.TypeObject, nil
	case "string":
		return genai.TypeString, nil
	case "integer":
		return genai.TypeInteger, nil
	case "number":
		return genai.TypeNumber, nil
	case "boolean":
		return genai.TypeBoolean, nil
	case "array":
		return genai.TypeArray, nil
	default:
		return "", fmt.Errorf("unsupported schema type %q", name)
	}
}

// orderProperties lists the required properties first, in the order the schema
// wrote them, then everything else alphabetically. Required arguments read
// first in the model's view of the tool, which is where they matter.
func orderProperties(node jsonSchema) []string {
	seen := map[string]bool{}
	order := make([]string, 0, len(node.Properties))
	for _, name := range node.Required {
		if _, ok := node.Properties[name]; ok && !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}
	rest := make([]string, 0, len(node.Properties))
	for name := range node.Properties {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	slices.Sort(rest)
	return append(order, rest...)
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

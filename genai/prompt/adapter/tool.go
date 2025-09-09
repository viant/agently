package adapter

import (
	"encoding/json"
	"strings"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
)

// ToToolDefinition converts an llm.Tool to a prompt.ToolDefinition suitable
// for prompt bindings. It marshals input/output schemas to json.RawMessage.
func ToToolDefinition(t llm.Tool) *prompt.ToolDefinition {
	name := strings.TrimSpace(t.Definition.Name)
	if name == "" {
		return nil
	}
	var inSchema, outSchema json.RawMessage
	if t.Definition.Parameters != nil {
		if b, err := json.Marshal(t.Definition.Parameters); err == nil {
			inSchema = b
		}
	}
	if t.Definition.OutputSchema != nil {
		if b, err := json.Marshal(t.Definition.OutputSchema); err == nil {
			outSchema = b
		}
	}
	return &prompt.ToolDefinition{
		Name:         name,
		Description:  t.Definition.Description,
		InputSchema:  inSchema,
		OutputSchema: outSchema,
	}
}

// ToToolDefinitions converts a slice of llm.Tool to prompt.ToolDefinition list,
// skipping entries that cannot be adapted.
func ToToolDefinitions(tools []llm.Tool) []*prompt.ToolDefinition {
	out := make([]*prompt.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		if def := ToToolDefinition(t); def != nil {
			out = append(out, def)
		}
	}
	return out
}

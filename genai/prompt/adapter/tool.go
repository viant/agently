package adapter

import (
	"github.com/viant/agently/genai/llm"
	mcpname "github.com/viant/agently/pkg/mcpname"
	"strings"
)

// ToToolDefinition converts an llm.Tool to an llm.ToolDefinition suitable
// for prompt bindings.
func ToToolDefinition(t llm.Tool) *llm.ToolDefinition {
	name := strings.TrimSpace(t.Definition.Name)
	if name == "" {
		return nil
	}
	// Canonicalize tool names to provider-safe form (service_path-method)
	name = mcpname.Canonical(name)
	// llm.ToolDefinition already uses structured maps; no need to re-marshal.
	return &llm.ToolDefinition{
		Name:         name,
		Description:  t.Definition.Description,
		Parameters:   t.Definition.Parameters,
		Required:     t.Definition.Required,
		OutputSchema: t.Definition.OutputSchema,
	}
}

// ToToolDefinitions converts a slice of llm.Tool to prompt.ToolDefinition list,
// skipping entries that cannot be adapted.
func ToToolDefinitions(tools []llm.Tool) []*llm.ToolDefinition {
	out := make([]*llm.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		if def := ToToolDefinition(t); def != nil {
			out = append(out, def)
		}
	}
	return out
}

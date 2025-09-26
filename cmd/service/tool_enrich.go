package service

import (
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/overlay"
	"github.com/viant/mcp-protocol/schema"
)

// enrichSchema merges the first matching overlay into base. It returns base
// unchanged when no overlay applies.
func enrichSchema(base map[string]any) map[string]any {
	if base == nil {
		return base
	}

	propsAny, ok := base["properties"]
	if !ok {
		return base
	}

	values, ok := propsAny.(schema.ToolInputSchemaProperties)
	if !ok {
		return base
	}

	var cloneProps = make(map[string]any, len(values))
	for k, v := range values {
		cloneProps[k] = v
	}

	for _, ov := range overlay.All() {
		if len(ov.Match.Fields) > 0 && !overlay.FieldsMatch(cloneProps, ov.Match.Fields, false) {
			continue
		}
		clone := make(map[string]any, len(base))
		clone["type"] = "object"
		clone["properties"] = cloneProps
		ov.Apply(cloneProps)
		return clone
	}
	return base
}

// EnrichToolDefinition mutates def in place, replacing its Parameters with an
// enriched copy when overlays match.
func (s *Service) EnrichToolDefinition(def *llm.ToolDefinition) {
	if def == nil {
		return
	}
	if params := def.Parameters; len(params) > 0 {
		def.Parameters = enrichSchema(params)
	}
}

// enriched returns a copy of defs with enriched parameter schemas.
func (s *Service) enriched(defs []llm.ToolDefinition) []llm.ToolDefinition {
	for i := range defs {
		if params := defs[i].Parameters; len(params) > 0 {
			defs[i].Parameters = enrichSchema(params)
		}
	}
	return defs
}

// EnrichedToolDefinitions exposes the executor definitions with overlay
// enrichment so that REST workspace handler returns UI-ready schemas.
func (s *Service) EnrichedToolDefinitions() []llm.ToolDefinition {
	base := s.ToolDefinitions()
	if len(base) == 0 {
		return base
	}
	out := make([]llm.ToolDefinition, len(base))
	copy(out, base)
	return s.enriched(out)
}

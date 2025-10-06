package service

import (
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/overlay"
	"github.com/viant/mcp-protocol/schema"
	"strings"
)

// enrichSchema merges the first matching overlay into base. It returns base
// unchanged when no overlay applies.
func enrichSchema(base map[string]any) map[string]any {
	if base == nil {
		return base
	}

	propsAny, ok := base["properties"]
	if !ok {
		// Still normalise defaults/types at root level if present
		fixSchemaNode(base)
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

	// Always fix nodes prior to applying overlay so UI widgets behave nicely
	fixSchemaNode(base)
	for _, ov := range overlay.All() {
		if len(ov.Match.Fields) > 0 && !overlay.FieldsMatch(cloneProps, ov.Match.Fields, false) {
			continue
		}
		clone := make(map[string]any, len(base))
		clone["type"] = "object"
		clone["properties"] = cloneProps
		ov.Apply(cloneProps)
		fixSchemaNode(clone)
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

// fixSchemaNode recursively normalises a JSON schema node so Forge SchemaBasedForm
// renders editable controls:
//   - ensure object defaults to {}
//   - ensure array defaults to []
//   - when array< string >, add x-ui-widget: tags for nicer UX
func fixSchemaNode(n map[string]any) {
	if n == nil {
		return
	}
	t, _ := n["type"].(string)
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "object" {
		if _, ok := n["default"]; !ok {
			n["default"] = map[string]any{}
		}
		if props, ok := n["properties"].(map[string]any); ok {
			for k, v := range props {
				if child, ok := v.(map[string]any); ok {
					fixSchemaNode(child)
					props[k] = child
				}
			}
		}
	} else if t == "array" {
		if _, ok := n["default"]; !ok {
			n["default"] = []any{}
		} else {
			// If mis-specified as object, coerce to [] for editability
			switch n["default"].(type) {
			case map[string]any:
				n["default"] = []any{}
			}
		}
		// Improve UX for string arrays
		if items, ok := n["items"].(map[string]any); ok {
			if itType, _ := items["type"].(string); strings.ToLower(strings.TrimSpace(itType)) == "string" {
				if _, ok := n["x-ui-widget"]; !ok {
					n["x-ui-widget"] = "tags"
				}
			} else {
				fixSchemaNode(items)
				n["items"] = items
			}
		}
	}
}

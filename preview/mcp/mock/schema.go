package mock

import "github.com/viant/mcp-protocol/schema"

// QueryInputSchema documents the shared request envelope for MCP tool discovery.
func QueryInputSchema() schema.ToolInputSchema {
	field := map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "object", "properties": map[string]any{"field": map[string]any{"type": "string"}, "alias": map[string]any{"type": "string"}, "aggregation": map[string]any{"type": "string"}}, "required": []string{"field"}}}}
	return schema.ToolInputSchema{Type: "object", Properties: map[string]map[string]interface{}{
		"variant": {"type": "string", "description": "Fixture overlay name, defaulting to the server variant"},
		"query": {"type": "object", "description": "Filter before projection, sort stably, then apply offset pagination. Report hosts additionally support declared aggregations.", "properties": map[string]any{
			"resultName": map[string]any{"type": "string", "description": "Select one named tabular result"},
			"projection": map[string]any{"type": "object", "properties": map[string]any{"fields": map[string]any{"type": "array", "items": field}, "dimensions": map[string]any{"type": "array", "items": field}, "measures": map[string]any{"type": "array", "items": field}}},
			"filter":     map[string]any{"type": "object", "description": "Nested and/or/not groups or field/op/value leaves: eq, neq, in, notIn, lt, lte, gt, gte, between, isNull, isNotNull, contains, startsWith, endsWith"},
			"orderBy":    map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"field": map[string]any{"type": "string"}, "direction": map[string]any{"enum": []string{"asc", "desc"}}, "nulls": map[string]any{"enum": []string{"first", "last"}}}, "required": []string{"field", "direction"}}},
			"page":       map[string]any{"type": "object", "properties": map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1}}, "required": []string{"limit"}},
		}},
	}}
}

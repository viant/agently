package executil

import (
	"encoding/json"

	plan "github.com/viant/agently/genai/llm"
)

// DecodeResults attempts to extract []plan.ToolCall from an arbitrary outVal
// returned by an orchestrated call. It handles both map[string]interface{}
// and typed structs by using a JSON round-trip for the latter.
func DecodeResults(outVal interface{}) []plan.ToolCall {
	if outVal == nil {
		return nil
	}
	// Fast-path: generic map
	if m, ok := outVal.(map[string]interface{}); ok {
		if v, exists := m["results"]; exists && v != nil {
			if b, e := json.Marshal(v); e == nil {
				var res []plan.ToolCall
				_ = json.Unmarshal(b, &res)
				return res
			}
		}
		return nil
	}
	// Fallback: JSON round-trip for typed outputs
	if b, e := json.Marshal(outVal); e == nil {
		var tmp struct {
			Results []plan.ToolCall `json:"results"`
		}
		_ = json.Unmarshal(b, &tmp)
		return tmp.Results
	}
	return nil
}

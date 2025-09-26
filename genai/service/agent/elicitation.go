package agent

import (
	"strings"

	"github.com/viant/agently/genai/agent/plan"
)

// missingRequired returns a list of required keys absent or empty in ctx.
func missingRequired(elicitation *plan.Elicitation, ctx map[string]any) []string {
	var out []string
	if elicitation == nil {
		return out
	}
	required := elicitation.RequestedSchema.Required
	if len(required) == 0 {
		return out
	}
	for _, key := range required {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if ctx == nil {
			out = append(out, key)
			continue
		}
		v, ok := ctx[key]
		if !ok || v == nil {
			out = append(out, key)
			continue
		}
		switch val := v.(type) {
		case string:
			if strings.TrimSpace(val) == "" {
				out = append(out, key)
			}
		}
	}
	return out
}

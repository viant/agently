package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/agent/plan"
	elact "github.com/viant/agently/genai/elicitation/action"
	"github.com/viant/agently/genai/memory"
)

// ensureInitialElicitation validates required context according to the agent's
// elicitation schema. When required fields are missing, it attempts to resolve
// them using auto-elicitation (when configured) and, if still missing and in
// interactive/server mode, records an elicitation prompt and waits for a
// resolution via the configured router/UI.
//
// It returns (proceed, error):
//   - proceed == true  -> continue with normal agent execution
//   - proceed == false -> stop processing (e.g., user declined or waiting already handled)
func (s *Service) ensureInitialElicitation(ctx context.Context, qi *QueryInput) (bool, error) {
	if qi == nil || qi.Agent == nil || qi.Agent.Elicitation == nil {
		return true, nil
	}

	// Compute missing fields against current context
	if missing := missingRequired(qi.Agent.Elicitation, qi.Context); len(missing) == 0 {
		return true, nil
	}

	// Auto-elicitation disabled for now; always proceed with interactive flow

	// Interactive flow: record and wait for user-provided payload via router/UI
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return false, fmt.Errorf("failed to get turn meta")
	}

	// Ensure elicitation has an ID; refine/message handled inside service
	req := cloneElicitation(qi.Agent.Elicitation)
	if strings.TrimSpace(req.ElicitationId) == "" {
		req.ElicitationId = uuid.New().String()
	}

	_, status, payload, err := s.elicitation.Elicit(ctx, &turn, "assistant", req)
	if err != nil {
		return false, err
	}
	if elact.Normalize(status) != elact.Accept {
		// User declined/cancelled; end turn without further processing
		return false, nil
	}

	// Merge accepted payload to context and persist
	if len(payload) > 0 {
		if qi.Context == nil {
			qi.Context = map[string]any{}
		}
		for k, v := range payload {
			qi.Context[k] = v
		}
		if err := s.updatedConversationContext(ctx, qi.ConversationID, qi); err != nil {
			return false, err
		}
	}
	return true, nil
}

// missingRequired returns a list of required keys absent or empty in ctx.
func missingRequired(elic *plan.Elicitation, ctx map[string]any) []string {
	var out []string
	if elic == nil {
		return out
	}
	required := elic.RequestedSchema.Required
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

// cloneElicitation returns a shallow copy of Elicitation sufficient for local use.
func cloneElicitation(in *plan.Elicitation) *plan.Elicitation {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

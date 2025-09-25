package plan

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	convctx "github.com/viant/agently/genai/conversation"
	mem "github.com/viant/agently/genai/memory"
	"github.com/viant/fluxor/model/types"
)

type Service struct {
	mu     sync.RWMutex
	byConv map[string]UpdatePlanPayload
}

// Name implements types.Service and identifies this Fluxor service.
func (s *Service) Name() string { return "core" }

// UpdatePlanInput matches the MCP tool-call arguments envelope.
// Example:
//
//	{
//	  "name": "update_plan",
//	  "call_id": "call_123",
//	  "arguments": "{\"explanation\":\"High-level release plan\",\"plan\":[{\"step\":\"Write release notes\",\"status\":\"in_progress\"},{\"step\":\"Bump versions\",\"status\":\"pending\"},{\"step\":\"Tag and publish\",\"status\":\"pending\"}]}"
//	}
type UpdatePlanInput struct {
	Name      string `json:"name,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments"`
}

// PlanItem represents a single step entry.
type PlanItem struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// UpdatePlanPayload is the JSON object stringified in UpdatePlanInput.Arguments.
type UpdatePlanPayload struct {
	Explanation string     `json:"explanation,omitempty"`
	Plan        []PlanItem `json:"plan"`
}

// UpdatePlanOutput echoes structured plan content for confirmation purposes.
type UpdatePlanOutput struct {
	CallID      string     `json:"call_id,omitempty"`
	Explanation string     `json:"explanation,omitempty"`
	Plan        []PlanItem `json:"plan"`
}

//go:embed description.txt
var description string

// Methods implements types.Service.
func (s *Service) Methods() types.Signatures {
	return types.Signatures{{
		Name:        "updatePlan",
		Description: description,
		Input:       reflect.TypeOf(&UpdatePlanInput{}),
		Output:      reflect.TypeOf(&UpdatePlanOutput{}),
	}}
}

// Method implements types.Service and returns the executable.
func (s *Service) Method(name string) (types.Executable, error) {
	if name != "updatePlan" {
		return nil, types.NewMethodNotFoundError(name)
	}
	return s.updatePlan, nil
}

// updatePlan parses the stringified JSON arguments and validates the plan.
func (s *Service) updatePlan(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*UpdatePlanInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*UpdatePlanOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	if input.Arguments == "" {
		return fmt.Errorf("arguments must be a non-empty JSON string")
	}

	var payload UpdatePlanPayload
	if err := json.Unmarshal([]byte(input.Arguments), &payload); err != nil {
		return fmt.Errorf("invalid arguments JSON: %w", err)
	}

	// Validate plan: allow only known statuses and at most one in_progress
	inProgress := 0
	for i, step := range payload.Plan {
		switch step.Status {
		case "pending", "in_progress", "completed":
		default:
			return fmt.Errorf("plan[%d]: invalid status %q", i, step.Status)
		}
		if step.Status == "in_progress" {
			inProgress++
		}
		if step.Step == "" {
			return fmt.Errorf("plan[%d]: step must be non-empty", i)
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("at most one step can be in_progress")
	}

	// Persist plan keyed by turn ID when available; otherwise by conversation ID.
	convID := convctx.ID(ctx)
	if convID == "" {
		convID = mem.ConversationIDFromContext(ctx)
	}
	s.mu.Lock()
	s.byConv[convID] = payload
	s.mu.Unlock()

	// Echo back the normalized payload
	output.CallID = input.CallID
	output.Explanation = payload.Explanation
	output.Plan = payload.Plan
	return nil
}

// New constructs the plan Service instance for registration.
func New() *Service { return &Service{byConv: map[string]UpdatePlanPayload{}} }

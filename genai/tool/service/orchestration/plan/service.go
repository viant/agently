package plan

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	svc "github.com/viant/agently/genai/tool/service"
)

type Service struct{}

// Name implements types.Service and identifies this service.
func (s *Service) Name() string { return "orchestration" }

// UpdatePlanInput matches the MCP tool-call arguments envelope.
type UpdatePlanInput struct {
	CallID string `json:"call_id,omitempty"`
	// Explanation: optional, short context for this update
	Explanation string `json:"explanation,omitempty"`
	// Plan: ordered list of steps, exactly one may be in_progress
	Plan []PlanItem `json:"plan"`
}

// PlanItem represents a single step entry.
type PlanItem struct {
	Step   string `json:"step"`
	Status string `json:"status"  choice:"pending" choice:"in_progress"  choice:"completed"`
}

// UpdatePlanPayload is the JSON object stringified in UpdatePlanInput.Arguments.
type UpdatePlanPayload struct {
	Explanation string     `json:"explanation"`
	Plan        []PlanItem `json:"plan"`
}

// UpdatePlanOutput echoes structured plan content for confirmation purposes.
type UpdatePlanOutput struct {
	CallID      string     `json:"call_id,omitempty"`
	Explanation string     `json:"explanation,omitempty"`
	Plan        []PlanItem `json:"plan"`
}

// EmptyInput used by status
type EmptyInput struct{}

//go:embed doc/update_plan.md
var description string

// Methods implements types.Service.
func (s *Service) Methods() svc.Signatures {
	return svc.Signatures{{
		Name:        "updatePlan",
		Description: description,
		Input:       reflect.TypeOf(&UpdatePlanInput{}),
		Output:      reflect.TypeOf(&UpdatePlanOutput{}),
	}, {
		Name:        "status",
		Description: "Returns the latest plan for the current conversation (if any)",
		Input:       reflect.TypeOf(&EmptyInput{}),
		Output:      reflect.TypeOf(&UpdatePlanOutput{}),
	}}
}

// Method implements types.Service and returns the executable.
func (s *Service) Method(name string) (svc.Executable, error) {
	switch name {
	case "updatePlan":
		return s.updatePlan, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// updatePlan parses the stringified JSON arguments and validates the plan.
func (s *Service) updatePlan(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*UpdatePlanInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*UpdatePlanOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}

	if input.CallID == "" {
		input.CallID = uuid.New().String()
	}
	var payload UpdatePlanPayload
	switch {
	case len(input.Plan) > 0 || input.Explanation != "":
		payload = UpdatePlanPayload{Explanation: input.Explanation, Plan: input.Plan}
	default:
		return fmt.Errorf("missing required parameters: provide 'plan' (and optional 'explanation')")
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

	// Echo back the normalized payload
	output.CallID = input.CallID
	output.Explanation = payload.Explanation
	output.Plan = payload.Plan
	return nil
}

// New constructs the plan Service instance for registration.
func New() *Service { return &Service{} }

// status returns the latest plan payload for the current conversation (best-effort).

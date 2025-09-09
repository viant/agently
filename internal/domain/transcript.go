package domain

import (
	"strings"

	"github.com/viant/agently/genai/agent/plan"
	msgread "github.com/viant/agently/internal/dao/message/read"
)

type Transcript []*msgread.MessageView

// Filter returns a new transcript containing only messages matching fn.
func (t Transcript) Filter(fn func(*msgread.MessageView) bool) Transcript {
	if len(t) == 0 {
		return t
	}
	out := make(Transcript, 0, len(t))
	for _, v := range t {
		if v == nil {
			continue
		}
		if fn == nil || fn(v) {
			out = append(out, v)
		}
	}
	return out
}

// WithoutInterim returns messages excluding interim entries (interim == 1).
func (t Transcript) WithoutInterim() Transcript {
	return t.Filter(func(v *msgread.MessageView) bool { return !v.IsInterim() })
}

// OnlyRoles returns messages with role matching any of the provided roles (case-insensitive).
func (t Transcript) OnlyRoles(roles ...string) Transcript {
	if len(roles) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, r := range roles {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "" {
			continue
		}
		set[r] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return t.Filter(func(v *msgread.MessageView) bool {
		_, ok := set[strings.ToLower(strings.TrimSpace(v.Role))]
		return ok
	})
}

// History returns user and assistant messages excluding interim and control type.
func (t Transcript) History() Transcript {
	return t.Filter(func(v *msgread.MessageView) bool {
		if v == nil || v.Type == "control" || v.IsInterim() {
			return false
		}
		role := strings.ToLower(strings.TrimSpace(v.Role))
		return role == "user" || role == "assistant"
	})
}

// Users returns user messages excluding interim.
func (t Transcript) Users() Transcript {
	return t.OnlyRoles("user").WithoutInterim()
}

// AssistantsNonInterim returns assistant messages excluding interim.
func (t Transcript) AssistantsNonInterim() Transcript {
	return t.Filter(func(v *msgread.MessageView) bool {
		if v == nil || v.IsInterim() || v.Type == "control" {
			return false
		}
		return strings.ToLower(strings.TrimSpace(v.Role)) == "assistant"
	})
}

// Outcomes flattens all aggregated outcomes across the transcript in order.
func (t Transcript) Outcomes() []*plan.Outcome {
	var out []*plan.Outcome
	for _, v := range t {
		if v == nil || len(v.Executions) == 0 {
			continue
		}
		out = append(out, v.Executions...)
	}
	return out
}

// StepOutcomes flattens step outcomes across all aggregated outcomes.
func (t Transcript) StepOutcomes() []*plan.StepOutcome {
	var steps []*plan.StepOutcome
	for _, oc := range t.Outcomes() {
		if oc == nil || len(oc.Steps) == 0 {
			continue
		}
		steps = append(steps, oc.Steps...)
	}
	return steps
}

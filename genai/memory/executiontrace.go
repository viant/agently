package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/viant/agently/genai/agent/plan"
)

// ExecutionTrace captures a single tool invocation (request + result) and
// optional execution-plan context. It is intended for auditing / UI inspection
// and is NOT sent back to the LLM as part of the conversation context.
type ExecutionTrace struct {
	// Auto-incremented per-conversation identifier.
	ID int `json:"id" yaml:"id"`

	// Parent assistant message ID that triggered this tool call.
	ParentMsgID string `json:"parentId" yaml:"parentId"`

	// Canonical service method, e.g. "file.count".
	Name string `json:"name" yaml:"name"`

	// Marshalled JSON argument object supplied to the tool.
	Request any `json:"request" yaml:"request"`

	// Whether the call succeeded.
	Success bool `json:"success" yaml:"success"`

	// Result (when Success==true) or nil.
	Result any `json:"result,omitempty" yaml:"result,omitempty"`

	// Error message when Success==false.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	StartedAt time.Time `json:"startedAt" yaml:"startedAt"`
	EndedAt   time.Time `json:"endedAt" yaml:"endedAt"`

	// Optional plan context – allows UI to tie tool invocation back to the
	// generating plan step.
	PlanID    string     `json:"planId,omitempty" yaml:"planId,omitempty"`
	StepIndex int        `json:"stepIndex,omitempty" yaml:"stepIndex,omitempty"`
	Step      *plan.Step `json:"step,omitempty" yaml:"step,omitempty"`
	Plan      *plan.Plan `json:"plan,omitempty" yaml:"plan,omitempty"`
	// When the step requested additional user input, surface the elicitation so
	// that callers can render an appropriate prompt.
	Elicitation *plan.Elicitation `json:"elicitation,omitempty" yaml:"elicitation,omitempty"`
}

// ExecutionStore is an in-memory registry of execution traces keyed by
// conversation ID.
type ExecutionStore struct {
	mux  sync.RWMutex
	data map[string][]*ExecutionTrace
}

// ListOutcome groups traces by PlanID and converts them into plan.Outcome
// structures that are easier to consume by UI clients. When format==outcome on
// the REST layer it calls this helper.
// The returned slice is sorted by the order the first trace of each plan was
// recorded (i.e. chronological plan execution order).
func (t *ExecutionStore) ListOutcome(ctx context.Context, convID string) ([]*plan.Outcome, error) {
	t.mux.RLock()
	defer t.mux.RUnlock()

	traces := t.data[convID]
	if len(traces) == 0 {
		return []*plan.Outcome{}, nil
	}

	// Group by PlanID while preserving insertion order.
	groups := make([]*plan.Outcome, 0)
	index := make(map[string]*plan.Outcome)

	for _, tr := range traces {
		if tr == nil {
			continue
		}
		pid := tr.PlanID
		if pid == "" {
			pid = "<unknown>"
		}
		out, ok := index[pid]
		if !ok {
			out = &plan.Outcome{ID: pid}
			index[pid] = out
			groups = append(groups, out)
		}

		reason := ""
		if tr.Step != nil {
			reason = tr.Step.Reason
		}
		// ensure Steps slice capacity
		stepOutcome := &plan.StepOutcome{
			ID:        fmt.Sprintf("%s-%d", pid, tr.StepIndex),
			Tool:      tr.Name,
			Reason:    reason,
			Request:   tr.Request,
			Response:  tr.Result,
			Success:   tr.Success,
			Error:     tr.Error,
			StartedAt: tr.StartedAt.Format(time.RFC3339),
			EndedAt:   tr.EndedAt.Format(time.RFC3339),
			Elapsed:   tr.EndedAt.Sub(tr.StartedAt).String(),
		}

		// Elicitation if any
		if tr.Elicitation != nil {
			stepOutcome.Elicited = map[string]interface{}{"message": tr.Elicitation.Message}
		}

		out.Steps = append(out.Steps, stepOutcome)
	}

	return groups, nil
}

// ListByParent returns a subset of traces for the supplied conversation filtered
// by the ParentMsgID. A nil slice is returned when the conversation ID is
// unknown or no trace matches the filter. The returned slice is a shallow copy
// and can be modified by the caller without affecting the store.
func (t *ExecutionStore) ListByParent(ctx context.Context, convID string, parentMsgID string) ([]*ExecutionTrace, error) {
	t.mux.RLock()
	defer t.mux.RUnlock()

	traces, ok := t.data[convID]
	if !ok || len(traces) == 0 {
		return []*ExecutionTrace{}, nil
	}

	// Collect matching entries.
	out := make([]*ExecutionTrace, 0, len(traces))
	for _, tr := range traces {
		if tr != nil && tr.ParentMsgID == parentMsgID {
			out = append(out, tr)
		}
	}
	return out, nil
}

// NewExecutionStore returns an empty store.
func NewExecutionStore() *ExecutionStore {
	return &ExecutionStore{data: make(map[string][]*ExecutionTrace)}
}

// Add appends a trace and assigns a sequential ID.
func (t *ExecutionStore) Add(ctx context.Context, convID string, trace *ExecutionTrace) (int, error) {
	if trace == nil {
		return 0, nil
	}
	t.mux.Lock()
	defer t.mux.Unlock()
	list := t.data[convID]
	trace.ID = len(list) + 1
	t.data[convID] = append(list, trace)
	return trace.ID, nil
}

// List returns a copy of all traces for conversation.
func (t *ExecutionStore) List(ctx context.Context, convID string) ([]*ExecutionTrace, error) {
	t.mux.RLock()
	defer t.mux.RUnlock()
	list := t.data[convID]
	out := make([]*ExecutionTrace, len(list))
	copy(out, list)
	return out, nil
}

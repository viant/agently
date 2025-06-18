package memory

import (
	"context"
	"sync"
	"time"

	"github.com/viant/agently/genai/agent/plan"
)

// ExecutionTrace captures a single tool invocation (request + result) and
// optional execution-plan context. It is intended for auditing / UI inspection
// and is NOT sent back to the LLM as part of the conversation context.
//
// (This struct was previously named ToolTrace; it has been renamed to reflect
// that the trace may cover additional execution information beyond the raw
// tool call.)
type ExecutionTrace struct {
	// Auto-incremented per-conversation identifier.
	ID int `json:"id" yaml:"id"`

	// Parent assistant message ID that triggered this tool call.
	ParentMsgID int `json:"parentId" yaml:"parentId"`

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

	// When the step requested additional user input, surface the elicitation so
	// that callers can render an appropriate prompt.
	Elicitation *plan.Elicitation `json:"elicitation,omitempty" yaml:"elicitation,omitempty"`
}

// TraceStore is an in-memory registry of execution traces keyed by
// conversation ID.
type TraceStore struct {
	mux  sync.RWMutex
	data map[string][]*ExecutionTrace
}

// NewTraceStore returns an empty store.
func NewTraceStore() *TraceStore {
	return &TraceStore{data: make(map[string][]*ExecutionTrace)}
}

// Add appends a trace and assigns a sequential ID.
func (t *TraceStore) Add(ctx context.Context, convID string, trace *ExecutionTrace) (int, error) {
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
func (t *TraceStore) List(ctx context.Context, convID string) ([]*ExecutionTrace, error) {
	t.mux.RLock()
	defer t.mux.RUnlock()
	list := t.data[convID]
	out := make([]*ExecutionTrace, len(list))
	copy(out, list)
	return out, nil
}

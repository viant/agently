package exec

import (
	"context"
	"time"

	executil "github.com/viant/agently/genai/extension/fluxor/llm/shared/executil"
	plan "github.com/viant/agently/genai/llm"
)

// NewTracer returns a Tracer that bridges to exec.Service trace methods.
func NewTracer(s *Service) executil.Tracer { return tracerAdapter{s: s} }

type tracerAdapter struct{ s *Service }

func (t tracerAdapter) UpdateTraceStart(ctx context.Context, conversationID string, traceID int, startAt time.Time) {
	if t.s != nil {
		t.s.updateTraceStart(ctx, conversationID, traceID, startAt)
	}
}

func (t tracerAdapter) UpdateTraceEnd(ctx context.Context, conversationID string, traceID int, result plan.ToolCall, duplicated bool, endAt time.Time) {
	if t.s != nil {
		t.s.updateTraceEnd(ctx, conversationID, traceID, result, duplicated, endAt)
	}
}

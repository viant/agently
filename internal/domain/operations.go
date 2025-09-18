package domain

import (
	"context"

	mcwrite "github.com/viant/agently/pkg/agently/modelcall"
	tcwrite "github.com/viant/agently/pkg/agently/toolcall"
)

// Operations exposes higher-level operations to record and fetch model/tool calls.
// This replaces the previous "Traces" terminology to better reflect that we
// persist concrete call operations (with optional payload indirection) rather
// than low-level tracing spans.
type Operations interface {
	// RecordModelCall stores a model call (with payload IDs already attached if any).
	RecordModelCall(ctx context.Context, call *mcwrite.ModelCall) error

	// RecordToolCall stores a tool call (with payload IDs already attached if any).
	RecordToolCall(ctx context.Context, call *tcwrite.ToolCall) error
}

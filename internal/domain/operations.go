package domain

import (
	"context"

	mcwrite "github.com/viant/agently/internal/dao/modelcall/write"
	tcwrite "github.com/viant/agently/internal/dao/toolcall/write"
)

// Operations exposes higher-level operations to record and fetch model/tool calls.
// This replaces the previous "Traces" terminology to better reflect that we
// persist concrete call operations (with optional payload indirection) rather
// than low-level tracing spans.
type Operations interface {
	// RecordModelCall stores a model call and optional request/response payloads.
	RecordModelCall(ctx context.Context, call *mcwrite.ModelCall, requestPayloadID, responsePayloadID string) error

	// RecordToolCall stores a tool call and optional request/response payloads.
	RecordToolCall(ctx context.Context, call *tcwrite.ToolCall, requestPayloadID, responsePayloadID string) error
}

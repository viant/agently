package llm

import (
	"context"
)

// StreamEvent represents a partial or complete event in a streaming LLM response.
// Response holds the generated partial or final content, Err indicates a streaming error.
type StreamEvent struct {
	Response *GenerateResponse
	Err      error
}

// StreamingModel is an optional interface for LLM providers that support streaming responses.
type StreamingModel interface {
	// Stream sends a chat request with streaming enabled and returns a channel of StreamEvent.
	Stream(ctx context.Context, request *GenerateRequest) (<-chan StreamEvent, error)
}

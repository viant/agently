package stream

import (
	"context"

	"github.com/viant/agently/genai/llm"
)

// Handler is invoked for every event in the session.
// Returning an error can be used to signal processing problems.
type Handler func(ctx context.Context, event *llm.StreamEvent) error

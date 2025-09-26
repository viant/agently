package elicitation

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
)

// noopAwaiter implements Awaiter and never contributes a result.
// It is used in non-interactive/server contexts where UI/router resolves elicitations.
type noopAwaiter struct{}

func (n *noopAwaiter) AwaitElicitation(ctx context.Context, req *plan.Elicitation) (*plan.ElicitResult, error) {
	return nil, nil
}

// NoopAwaiterFactory returns a factory that produces noop awaiters.
func NoopAwaiterFactory() func() Awaiter { return func() Awaiter { return &noopAwaiter{} } }

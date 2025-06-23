package agently

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/io/elicitation/stdio"
	"io"
	"os"
)

// stdinAwaiter prompts the user on stdin/stdout whenever the runtime requests
// a payload that must satisfy the supplied JSON-schema. The implementation
// delegates to stdio.Prompt so the prompting logic remains in a single unit-
// tested place.
type stdinAwaiter struct{}

// AwaitElicitation implements elicitation.Awaiter.
func (a *stdinAwaiter) AwaitElicitation(ctx context.Context, req *plan.Elicitation) (*plan.ElicitResult, error) {
	var w io.Writer = os.Stdout // ensure stdout flushing
	return stdio.Prompt(ctx, w, os.Stdin, req)
}

// newStdinAwaiter is a helper for neat option wiring.
func newStdinAwaiter() elicitation.Awaiter { return &stdinAwaiter{} }

// Compile-time assertion.
var _ elicitation.Awaiter = (*stdinAwaiter)(nil)

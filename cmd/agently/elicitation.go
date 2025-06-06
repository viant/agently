package agently

// Package agently implements the CLI front-end for the Agently executor.

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/io/elicitation/stdio"
	"io"
	"os"
)

// stdinAwaiter prompts the user on stdin/stdout whenever the runtime requests
// a payload that must satisfy the supplied JSON-schema.
//
// The implementation purposely keeps the interaction minimal so that it works
// in both TTY and pipe scenarios: the schema (when present) is printed and the
// user is asked to either enter a single-line JSON document (accepted) or hit
// ENTER without input (decline).
type stdinAwaiter struct{}

// AwaitElicitation delegates to the shared stdio.Prompt helper so that all
// interactive behaviour is implemented in a single, unit-tested place.
// It uses os.Stdout / os.Stdin as the I/O channels while honouring ctx for
// cancellation.
func (a *stdinAwaiter) AwaitElicitation(ctx context.Context, req *plan.Elicitation) (*plan.ElicitResult, error) {
	// stdio.Prompt writes directly to stdout and reads from stdin. Wrap stdout
	// with an io.Writer that flushes per Write to preserve ordering when the
	// console is buffered.
	var w io.Writer = os.Stdout
	return stdio.Prompt(ctx, w, os.Stdin, req)
}

// newStdinAwaiter returns an instance that satisfies the generic Awaiter
// interface.
func newStdinAwaiter() elicitation.Awaiter { return &stdinAwaiter{} }

// Ensure compile-time interface satisfaction.
var _ elicitation.Awaiter = (*stdinAwaiter)(nil)

// init wires the awaiter into the executor singleton via the helper defined in
// shared.go so that all CLI commands automatically benefit from interactive
// schema prompts.
func init() {
	registerExecOption(executor.WithElicitationAwaiter(newStdinAwaiter()))
}

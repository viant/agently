package elicitation

// Package elicitation groups types that deal with interactive collection of a
// JSON-document that fulfils a given JSON-schema.

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
)

// Awaiter waits for a user (or other UI component) to either provide a
// payload that satisfies the supplied schema or decline the request.
//
// The interface is intentionally minimal so that it can be implemented by a
// variety of front-ends (CLI, HTTP, GUI, tests, …) without dragging in large
// dependencies.
type Awaiter interface {
	// AwaitElicitation blocks until the user accepts (or declines) the
	// elicitation request. It must honour ctx for cancellation.
	AwaitElicitation(ctx context.Context, p *plan.Elicitation) (*plan.ElicitResult, error)
}

package tool

import (
	"context"
	"io"

	"github.com/viant/agently/genai/llm"
)

// ---------------------------------------------------------------------------
// Registry abstraction
// ---------------------------------------------------------------------------

// Handler executes a tool call and returns its textual result.
type Handler func(ctx context.Context, args map[string]interface{}) (string, error)

// Registry defines the minimal interface required by the rest of the
// code-base.  Previous consumers used a concrete *Registry struct; moving to an
// interface allows alternative implementations (remote catalogues, mocks,
// etc.) while retaining backward-compatibility.
type Registry interface {
	// Definitions returns the merged list of available tool definitions.
	Definitions() []llm.ToolDefinition

	//MatchDefinition matches tool definition based on pattern
	MatchDefinition(pattern string) []*llm.ToolDefinition

	// GetDefinition fetches the definition for the given tool name. The second
	// result value indicates whether the definition exists.
	GetDefinition(name string) (*llm.ToolDefinition, bool)

	// MustHaveTools converts a set of patterns into the LLM toolkit slice used by
	// generation prompts.
	MustHaveTools(patterns []string) ([]llm.Tool, error)

	// Execute invokes the given tool with the supplied arguments and returns
	// its textual result.
	Execute(ctx context.Context, name string, args map[string]interface{}) (string, error)

	// SetDebugLogger attaches a writer that receives every executed tool call
	// for debugging.
	SetDebugLogger(w io.Writer)
}

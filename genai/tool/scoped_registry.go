package tool

import (
	"context"
	"io"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
)

// scopedRegistry is a lightweight wrapper that binds a tool.Registry to a
// specific conversation ID. It ensures Execute calls always carry the
// conversation identifier in context so downstream adapters (e.g., MCP client
// manager) can resolve per-conversation resources.
type scopedRegistry struct {
	inner  Registry
	convID string
}

// WithConversation returns a Registry that guarantees ctx carries convID for
// every Execute call. All other methods delegate to the underlying registry.
func WithConversation(inner Registry, convID string) Registry {
	if inner == nil || convID == "" {
		// No-op wrapper when missing dependencies; return the original registry
		// to preserve backward compatibility.
		return inner
	}
	return &scopedRegistry{inner: inner, convID: convID}
}

// Definitions delegates to the underlying registry.
func (s *scopedRegistry) Definitions() []llm.ToolDefinition { return s.inner.Definitions() }

// MatchDefinition delegates to the underlying registry.
func (s *scopedRegistry) MatchDefinition(pattern string) []*llm.ToolDefinition {
	return s.inner.MatchDefinition(pattern)
}

// GetDefinition delegates to the underlying registry.
func (s *scopedRegistry) GetDefinition(name string) (*llm.ToolDefinition, bool) {
	return s.inner.GetDefinition(name)
}

// MustHaveTools delegates to the underlying registry.
func (s *scopedRegistry) MustHaveTools(patterns []string) ([]llm.Tool, error) {
	return s.inner.MustHaveTools(patterns)
}

// Execute injects the conversation ID into context and delegates to the
// underlying registry.
func (s *scopedRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if s.convID != "" {
		if memory.ModelMessageIDFromContext(ctx) == "" {
			ctx = memory.WithConversationID(ctx, s.convID)
		}
	}
	return s.inner.Execute(ctx, name, args)
}

// SetDebugLogger delegates to the underlying registry.
func (s *scopedRegistry) SetDebugLogger(w io.Writer) { s.inner.SetDebugLogger(w) }

// Initialize delegates to the underlying registry.
func (s *scopedRegistry) Initialize(ctx context.Context) { s.inner.Initialize(ctx) }

package mcp

import (
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/io/elicitation"
)

// Option configures the customised client.
type Option func(*Client)

// WithAwaiter sets the interactive prompt handler used by orchestrating code
// whenever the server sends an "elicitation" request that needs user input.
func WithAwaiter(a elicitation.Awaiter) Option {
	return func(cl *Client) { cl.awaiter = a }
}

// WithLLMCore stores a reference to the shared LLM core so that future
// versions can route tool calls through the same usage accounting / logging
// facilities. The current implementation keeps the value for completeness but
// does not actively use it yet.
func WithLLMCore(c *core.Service) Option {
	return func(cl *Client) { cl.llmCore = c }
}

// WithURLOpener overrides the function used to open browser.
func WithURLOpener(fn func(string) error) Option {
	return func(c *Client) { c.openURLFn = fn }
}

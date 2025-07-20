package mcp

import (
	awaitreg "github.com/viant/agently/genai/awaitreg"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/memory"
)

// Option configures the customised client.
type Option func(*Client)

// WithAwaiters sets the interactive prompt handler used by orchestrating code
// whenever the server sends an "elicitation" request that needs user input.
// WithAwaiter is a backward-compatibility helper that accepts a factory function
// and internally creates an Awaiters registry. Existing call-sites that passed
// only a constructor (func() Awaiter) can keep using this option whilst the
// implementation migrates towards the registry pattern.
func WithAwaiter(f func() elicitation.Awaiter) Option {
	return func(cl *Client) {
		if f == nil {
			return
		}
		cl.awaiters = awaitreg.New(f)
	}
}

// WithRegistry injects an already-initialised per-conversation awaiter
// registry.
func WithRegistry(r *awaitreg.Registry) Option {
	return func(cl *Client) { cl.awaiters = r }
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

// WithHistory injects a shared conversation history implementation so that
// the client can persist interactive elicitation messages even when the
// incoming context does not carry the history reference (e.g. when the
// runtime originates from a background goroutine that lost values).
func WithHistory(h memory.History) Option {
	return func(c *Client) { c.history = h }
}

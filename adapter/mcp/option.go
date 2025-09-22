package mcp

import (
	"github.com/viant/agently/adapter/mcp/router"
	apiconv "github.com/viant/agently/client/conversation"
	awaitreg "github.com/viant/agently/genai/awaitreg"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/service/core"
	elicsvc "github.com/viant/agently/genai/service/elicitation"
	"github.com/viant/mcp-protocol/schema"
)

// Option configures the customised client.
type Option func(*Client)

// WithAwaiters sets the interactive prompt handler used by orchestrating code
// whenever the server sends an "elicitation" request that needs user input.
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

// WithRouter registers a global elicitation router used when no Awaiters are provided.
func WithRouter(r *router.Router) Option {
	return func(c *Client) { c.router = r }
}

// WithRefinerService injects a refiner service applied to incoming elicitation requests.
func WithRefinerService(svc interface {
	RefineRequestedSchema(rs *schema.ElicitRequestParamsRequestedSchema)
}) Option {
	return func(c *Client) {
		// wrap via function closure in Elicit – store via a tiny adapter
		cRefine := func(rs *schema.ElicitRequestParamsRequestedSchema) {
			if svc != nil {
				svc.RefineRequestedSchema(rs)
			}
		}
		// monkey-patch by replacing default preset call via a local hook
		c.onRefine = cRefine
		// Also configure elicitation service if present
		if c.elicition != nil && svc != nil {
			c.elicition.SetRefiner(svc)
		}
	}
}

// WithConversationClient injects conversation client to persist elicitation messages (pollable by web UI).
func WithConversationClient(cli apiconv.Client) Option {
	return func(c *Client) {
		c.convClient = cli
		if cli != nil {
			c.elicition = elicsvc.New(cli)
		}
	}
}

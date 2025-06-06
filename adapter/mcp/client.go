package mcp

// Client implements the mcp-protocol client-side Operations interface.  It is
// *not* a network client – instead it adapts protocol requests into local
// Agently capabilities (LLM generation, browser interaction, etc.).

import (
	"context"
	"encoding/json"
	elicitationSchema "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/conv"
	"github.com/viant/jsonrpc"
	"github.com/viant/mcp-protocol/schema"
)

// Client adapts MCP operations to local execution.
type Client struct {
	core       *core.Service
	openURLFn  func(string) error
	implements map[string]bool
	awaiter    elicitation.Awaiter
	llmCore    *core.Service
}

func (c *Client) Init(ctx context.Context, capabilities *schema.ClientCapabilities) {
	if capabilities.Elicitation != nil {
		c.implements[schema.MethodElicitationCreate] = true
	}
	if capabilities.Roots != nil {
		c.implements[schema.MethodRootsList] = true
	}
	if capabilities.UserInteraction != nil {
		c.implements[schema.MethodInteractionCreate] = true
	}
	if capabilities.Sampling != nil {
		c.implements[schema.MethodSamplingCreateMessage] = true
	}
}

// Implements tells dispatcher which methods we support.
func (c *Client) Implements(method string) bool {
	switch method {
	case schema.MethodRootsList,
		schema.MethodSamplingCreateMessage,
		schema.MethodElicitationCreate,
		schema.MethodInteractionCreate:
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// Operations
// -----------------------------------------------------------------------------

func (c *Client) ListRoots(ctx context.Context, p *schema.ListRootsRequestParams) (*schema.ListRootsResult, *jsonrpc.Error) {
	// For local execution we have no workspace roots; return empty.
	return &schema.ListRootsResult{Roots: []schema.Root{}}, nil
}

func (c *Client) CreateUserInteraction(ctx context.Context, p *schema.CreateUserInteractionRequestParams) (*schema.CreateUserInteractionResult, *jsonrpc.Error) {
	if p == nil || p.Interaction.Url == "" {
		return nil, jsonrpc.NewInvalidParamsError("uri is required", nil)
	}
	_ = c.openURLFn(p.Interaction.Url) // ignore error – non-critical
	return &schema.CreateUserInteractionResult{}, nil
}

func (c *Client) Elicit(ctx context.Context, params *schema.ElicitRequestParams) (*schema.ElicitResult, *jsonrpc.Error) {
	// When an Awaiter is configured we bypass the network round-trip entirely
	// and resolve the prompt locally. We must translate between the MCP
	// protocol types and the lightweight types declared in the local
	// agently/schema package.
	if c.awaiter != nil {
		// ------------------------------------------------------------------
		// Build JSON-schema string from the restricted MCP subset so that the
		// generic Awaiter can operate on a single schema document.
		// ------------------------------------------------------------------
		var schemaJSON string
		{
			doc := map[string]interface{}{
				"type":       params.RequestedSchema.Type,
				"properties": params.RequestedSchema.Properties,
			}
			if len(params.RequestedSchema.Required) > 0 {
				doc["required"] = params.RequestedSchema.Required
			}
			if b, _ := json.Marshal(doc); len(b) > 0 {
				schemaJSON = string(b)
			}
		}

		localReq := &elicitationSchema.Elicitation{Schema: schemaJSON}
		res, err := c.awaiter.AwaitElicitation(ctx, localReq)
		if err != nil {
			return nil, jsonrpc.NewInternalError(err.Error(), nil)
		}

		// Map back to MCP result ------------------------------------
		return &schema.ElicitResult{
			Action:  schema.ElicitResultAction(res.Action),
			Content: res.Payload,
		}, nil
	}
	return nil, jsonrpc.NewInternalError("elicitation awaiter wes not configured ", nil)
}

func (c *Client) CreateMessage(ctx context.Context, p *schema.CreateMessageRequestParams) (*schema.CreateMessageResult, *jsonrpc.Error) {
	if c.core == nil {
		return nil, jsonrpc.NewInternalError("llm core not configured", nil)
	}
	if p == nil {
		return nil, jsonrpc.NewInvalidParamsError("params is nil", nil)
	}

	// Use last message as prompt, earlier messages ignored in MVP.
	var prompt string
	if len(p.Messages) > 0 {
		prompt = p.Messages[len(p.Messages)-1].Content.Text // assuming text field
	}
	in := &core.GenerateInput{
		Preferences:  llm.NewModelPreferences(llm.WithPreferences(p.ModelPreferences)),
		Prompt:       prompt,
		SystemPrompt: conv.Dereference[string](p.SystemPrompt),
	}
	if p.MaxTokens > 0 || p.Temperature != nil || len(p.StopSequences) > 0 {
		in.Options = &llm.Options{
			MaxTokens:   p.MaxTokens,
			Temperature: conv.Dereference[float64](p.Temperature),
			StopWords:   p.StopSequences,
		}
	}

	var out core.GenerateOutput
	if err := c.core.Generate(ctx, in, &out); err != nil {
		return nil, jsonrpc.NewInternalError(err.Error(), nil)
	}

	result := &schema.CreateMessageResult{
		Model: in.Model,
		Role:  schema.RoleAssistant,
		Content: schema.CreateMessageResultContent{
			Type: "text",
			Text: out.Content,
		},
	}
	return result, nil
}

func (c *Client) asErr(e error) *jsonrpc.Error {
	if e == nil {
		return nil
	}
	return jsonrpc.NewInternalError(e.Error(), nil)
}

// Option type remains in option.go

// NewClient returns a ready client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		openURLFn:  defaultOpenURL,
		implements: make(map[string]bool),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

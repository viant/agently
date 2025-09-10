package mcp

// Client implements the mcp-protocol client-side Operations interface.  It is
// *not* a network client – instead it adapts protocol requests into local
// Agently capabilities (LLM generation, browser interaction, etc.).

import (
	"context"

	awaitreg "github.com/viant/agently/genai/awaitreg"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"

	"sync"

	"github.com/google/uuid"
	elicitationSchema "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation/refiner"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/llm"

	"strings"

	"github.com/viant/agently/internal/conv"
	"github.com/viant/jsonrpc"
	"github.com/viant/mcp-protocol/schema"
)

var waiterRegistry sync.Map // msgID -> chan *schema.ElicitResult

var interactionWaiterRegistry sync.Map // msgID -> chan *schema.CreateUserInteractionResult

// Waiter returns the registered channel for a given message ID, if present.
// It is used by the HTTP callback handler to deliver the user's response and
// unblock the goroutine waiting inside Client.Elicit.
func Waiter(id string) (chan *schema.ElicitResult, bool) {
	v, ok := waiterRegistry.Load(id)
	if !ok {
		return nil, false
	}
	return v.(chan *schema.ElicitResult), true
}

// Client adapts MCP operations to local execution.
type Client struct {
	core       *core.Service
	openURLFn  func(string) error
	implements map[string]bool
	awaiters   *awaitreg.Registry
	llmCore    *core.Service
	history    memory.History // optional fallback when ctx lacks history
	// waiter registries are shared for elicitation and interaction
}

func (*Client) LastRequestID() jsonrpc.RequestId {
	return 0
}

func (*Client) NextRequestID() jsonrpc.RequestId {
	return 0
}

func (c *Client) Init(ctx context.Context, capabilities *schema.ClientCapabilities) {
	if capabilities.Elicitation != nil {
		c.implements[schema.MethodElicitationCreate] = true
	}
	if capabilities.Roots != nil {
		c.implements[schema.MethodRootsList] = true
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
		schema.MethodElicitationCreate:
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// Operations
// -----------------------------------------------------------------------------

func (c *Client) ListRoots(ctx context.Context, p *jsonrpc.TypedRequest[*schema.ListRootsRequest]) (*schema.ListRootsResult, *jsonrpc.Error) {
	// For local execution we have no workspace roots; return empty.
	return &schema.ListRootsResult{Roots: []schema.Root{}}, nil
}

func (c *Client) Elicit(ctx context.Context, request *jsonrpc.TypedRequest[*schema.ElicitRequest]) (*schema.ElicitResult, *jsonrpc.Error) {
	params := request.Request.Params
	if c.awaiters != nil {
		localReq := &elicitationSchema.Elicitation{ElicitRequestParams: params}

		// Handle out-of-band URL prompt: try to open browser before invoking
		// Awaiter so the user can fill the form.
		if strings.TrimSpace(params.Url) != "" && c.openURLFn != nil {
			_ = c.openURLFn(params.Url)
		}

		aw := c.awaiters.Ensure(conversation.ID(ctx))

		res, err := aw.AwaitElicitation(ctx, localReq)
		if err != nil {
			return nil, jsonrpc.NewInternalError(err.Error(), nil)
		}

		// Map back to MCP result ------------------------------------
		return &schema.ElicitResult{
			Action:  schema.ElicitResultAction(res.Action),
			Content: res.Payload,
		}, nil
	}
	// ------------------------------------------------------------------
	// No local awaiters – persist message and block until HTTP callback
	// resolves it through waiterRegistry (web UI scenario).
	// ------------------------------------------------------------------
	// ------------------------------------------------------------------
	// Generate synthetic message ID so that HTTP callback can reference it
	msgID := uuid.New().String()

	history := c.history
	if history != nil {

		owner, err := history.LatestMessage(ctx)
		if err != nil {
			return nil, jsonrpc.NewInternalError(err.Error(), nil)
		}

		parentID := owner.ParentID
		if parentID == "" {
			parentID = owner.ID
		}

		_ = history.AddMessage(ctx, memory.Message{
			ID:             msgID,
			ParentID:       parentID,
			ConversationID: owner.ConversationID,
			Role:           "mcpelicitation",
			Content:        params.Message,
			Elicitation: func() *elicitationSchema.Elicitation {
				// Refine schema before persisting so that UI sees improved version.
				refiner.Refine(&params.RequestedSchema)
				return &elicitationSchema.Elicitation{ElicitRequestParams: params}
			}(),
			CallbackURL: "/elicitation/" + msgID,
			Status:      "open",
		})
	}

	// Register waiter and block
	ch := make(chan *schema.ElicitResult, 1)
	waiterRegistry.Store(msgID, ch)
	select {
	case res := <-ch:
		waiterRegistry.Delete(msgID)
		return res, nil
	case <-ctx.Done():
		waiterRegistry.Delete(msgID)
		return nil, jsonrpc.NewInternalError("elicitation cancelled", nil)
	}
}

func (c *Client) CreateMessage(ctx context.Context, request *jsonrpc.TypedRequest[*schema.CreateMessageRequest]) (*schema.CreateMessageResult, *jsonrpc.Error) {
	if c.core == nil {
		return nil, jsonrpc.NewInternalError("llm core not configured", nil)
	}
	if request.Request == nil {
		return nil, jsonrpc.NewInvalidParamsError("params is nil", nil)
	}
	p := &request.Request.Params
	// Use last message as prompt, earlier messages ignored in MVP.
	var promptText string
	if len(p.Messages) > 0 {
		promptText = p.Messages[len(p.Messages)-1].Content.Text // assuming text field
	}
	in := &core.GenerateInput{
		ModelSelection: llm.ModelSelection{
			Preferences: llm.NewModelPreferences(llm.WithPreferences(p.ModelPreferences)),
		},
		Prompt:       &prompt.Prompt{Text: promptText},
		SystemPrompt: &prompt.Prompt{Text: conv.Dereference[string](p.SystemPrompt)},
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

func (c *Client) Notify(ctx context.Context, notification *jsonrpc.Notification) error {
	return nil
}

func (c *Client) OnNotification(ctx context.Context, notification *jsonrpc.Notification) {

}

func (c *Client) ProtocolVersion() string {
	//schema.LatestProtocolVersion
	return "2025-06-18"
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

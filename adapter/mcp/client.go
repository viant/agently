package mcp

// Client implements the mcp-protocol client-side Operations interface.  It is
// *not* a network client – instead it adapts protocol requests into local
// Agently capabilities (LLM generation, browser interaction, etc.).

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	elicitationSchema "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/io/elicitation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"

	"github.com/viant/agently/internal/conv"
	"github.com/viant/jsonrpc"
	"github.com/viant/mcp-protocol/schema"
)

type ctxKey string

const historyKey ctxKey = "historyStore"

func historyFromContext(ctx context.Context) memory.History {
	if v := ctx.Value(historyKey); v != nil {
		if h, ok := v.(memory.History); ok {
			return h
		}
	}
	return nil
}

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

// WaiterInteraction returns registered channel for a user-interaction message
// (if any) so that HTTP callback can deliver user decisions back to the MCP
// client.
func WaiterInteraction(id string) (chan *schema.CreateUserInteractionResult, bool) {
	v, ok := interactionWaiterRegistry.Load(id)
	if !ok {
		return nil, false
	}
	return v.(chan *schema.CreateUserInteractionResult), true
}

// Client adapts MCP operations to local execution.
type Client struct {
	core       *core.Service
	openURLFn  func(string) error
	implements map[string]bool
	awaiter    elicitation.Awaiter
	llmCore    *core.Service

	history memory.History // optional fallback when ctx lacks history

	// waiter registries are shared for elicitation and interaction
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

func (c *Client) ListRoots(ctx context.Context, p *jsonrpc.TypedRequest[*schema.ListRootsRequest]) (*schema.ListRootsResult, *jsonrpc.Error) {
	// For local execution we have no workspace roots; return empty.
	return &schema.ListRootsResult{Roots: []schema.Root{}}, nil
}

func (c *Client) CreateUserInteraction(ctx context.Context, request *jsonrpc.TypedRequest[*schema.CreateUserInteractionRequest]) (*schema.CreateUserInteractionResult, *jsonrpc.Error) {
	if request == nil || request.Request == nil || request.Request.Params.Interaction.Url == "" {
		return nil, jsonrpc.NewInvalidParamsError("url is required", nil)
	}

	p := request.Request.Params

	// Determine history presence to decide CLI vs Web flow
	var hist memory.History
	if h := historyFromContext(ctx); h != nil {
		hist = h
	} else if c.history != nil {
		hist = c.history
	}

	// CLI (no history) – open URL immediately and accept
	if hist == nil {
		if c.openURLFn != nil {
			_ = c.openURLFn(p.Interaction.Url)
		}
		return &schema.CreateUserInteractionResult{}, nil
	}

	// Web flow ------------------------------------------------------
	convID := memory.ConversationIDFromContext(ctx)
	var parentID string
	if convID == "" {
		if cid, msg, err := hist.LatestMessage(ctx); err == nil && msg != nil {
			convID = cid
			parentID = msg.ID
		}
	}
	if convID == "" {
		return nil, jsonrpc.NewInternalError("unable to resolve conversation id", nil)
	}

	msgID := uuid.New().String()
	_ = hist.AddMessage(ctx, convID, memory.Message{
		ID:          msgID,
		ParentID:    parentID,
		Role:        "mcpuserinteraction",
		Interaction: &memory.UserInteraction{URL: p.Interaction.Url},
		CallbackURL: "/interaction/" + msgID,
		Status:      "open",
	})

	ch := make(chan *schema.CreateUserInteractionResult, 1)
	interactionWaiterRegistry.Store(msgID, ch)
	select {
	case res := <-ch:
		interactionWaiterRegistry.Delete(msgID)
		return res, nil
	case <-ctx.Done():
		interactionWaiterRegistry.Delete(msgID)
		return nil, jsonrpc.NewInternalError("interaction cancelled", nil)
	}
}

func (c *Client) Elicit(ctx context.Context, request *jsonrpc.TypedRequest[*schema.ElicitRequest]) (*schema.ElicitResult, *jsonrpc.Error) {
	params := request.Request.Params
	if c.awaiter != nil {
		localReq := &elicitationSchema.Elicitation{ElicitRequestParams: params}
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
	// ------------------------------------------------------------------
	// No local awaiter – persist message and block until HTTP callback
	// resolves it through waiterRegistry (web UI scenario).
	// ------------------------------------------------------------------
	// ------------------------------------------------------------------
	// Generate synthetic message ID so that HTTP callback can reference it
	msgID := uuid.New().String()

	history := c.history
	if history != nil {
		convID := memory.ConversationIDFromContext(ctx)
		var parentID string
		if convID == "" {
			if cid, msg, err := history.LatestMessage(ctx); err == nil && msg != nil {
				convID = cid
				parentID = msg.ID
				if msg.ParentID != "" {
					parentID = msg.ParentID
				}
			}
		}
		if convID != "" {
			fmt.Printf("Adding mcpelicitation %v %v\n ", convID, parentID)
			_ = history.AddMessage(ctx, convID, memory.Message{
				ID:          msgID,
				ParentID:    parentID,
				Role:        "mcpelicitation",
				Content:     params.Message,
				Elicitation: &elicitationSchema.Elicitation{ElicitRequestParams: params},
				CallbackURL: "/elicitation/" + msgID,
				Status:      "open",
			})
		}
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

func (c *Client) CreateMessage(ctx context.Context, reqeust *jsonrpc.TypedRequest[*schema.CreateMessageRequest]) (*schema.CreateMessageResult, *jsonrpc.Error) {
	if c.core == nil {
		return nil, jsonrpc.NewInternalError("llm core not configured", nil)
	}
	if reqeust.Request == nil {
		return nil, jsonrpc.NewInvalidParamsError("params is nil", nil)
	}
	p := &reqeust.Request.Params
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

func (c *Client) Notify(ctx context.Context, notification *jsonrpc.Notification) error {
	return nil
}

func (c *Client) OnNotification(ctx context.Context, notification *jsonrpc.Notification) {

}

func (c *Client) ProtocolVersion() string {
	//schema.LatestProtocolVersion
	return "2025-03-27"
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

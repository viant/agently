package mcp

// Client implements the mcp-protocol client-side Operations interface.  It is
// *not* a network client – instead it adapts protocol requests into local
// Agently capabilities (LLM generation, browser interaction, etc.).

import (
	"context"
	"fmt"

	"github.com/viant/agently/genai/prompt"
	core2 "github.com/viant/agently/genai/service/core"

	"sync"

	elicitationSchema "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"

	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/elicitation"
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
	core        *core2.Service
	openURLFn   func(string) error
	implements  map[string]bool
	llmCore     *core2.Service
	convID      string
	convClient  apiconv.Client
	elicitation *elicitation.Service
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
	// Refine, persist, then delegate wait to the elicitation service
	params := request.Request.Params
	c.elicitation.RefineRequestedSchema(&params.RequestedSchema)
	convID := c.convID
	if strings.TrimSpace(convID) == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if err := c.persistElicitationMessage(ctx, &params, request.Id); err != nil {
		return nil, jsonrpc.NewInternalError(fmt.Sprintf("failed to persist elicitation: %v", err), nil)
	}
	status, payload, err := c.elicitation.Wait(ctx, convID, params.ElicitationId)
	if err != nil {
		return nil, jsonrpc.NewInternalError(fmt.Sprintf("elicitation wait failed: %v", err), nil)
	}
	return &schema.ElicitResult{Action: schema.ElicitResultAction(status), Content: payload}, nil
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
	in := &core2.GenerateInput{
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

	var out core2.GenerateOutput
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

// persistElicitationMessage best-effort persists an assistant message with
// elicitation payload so that poll-based UIs can display it while awaiting user action.
func (c *Client) persistElicitationMessage(ctx context.Context, params *schema.ElicitRequestParams, rpcID uint64) error {
	if strings.TrimSpace(c.convID) == "" {
		return fmt.Errorf("convID is required")
	}
	if strings.TrimSpace(params.ElicitationId) == "" {
		params.ElicitationId = uuid.New().String()
	}
	payload := &elicitationSchema.Elicitation{ElicitRequestParams: *params}
	// Provide a direct callback URL using conversation + elicitationId for unified posting.
	payload.CallbackURL = fmt.Sprintf("/v1/api/conversations/%s/elicitation/%s", c.convID, params.ElicitationId)
	aConversation, err := c.convClient.GetConversation(ctx, c.convID)
	if err != nil {
		return fmt.Errorf("failed to get conversation: %v for elicitation, %v", err, params.ElicitationId)
	}
	turn := &memory.TurnMeta{ConversationID: c.convID}
	if aConversation.LastTurnId != nil {
		turn.TurnID = *aConversation.LastTurnId
		turn.ParentMessageID = *aConversation.LastTurnId
	}
	_, err = c.elicitation.Record(ctx, turn, "tool", payload)
	return err
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

// NewClient returns a ready client with explicit dependencies.
// - el: elicitation service (required)
// - conv: conversation client used for direct updates (required)
// - newAwaiter: factory for interactive prompts in CLI; pass nil for server mode
// - openURL: override URL opener; pass nil to use default or to disable
func NewClient(el *elicitation.Service, conv apiconv.Client, openURL func(string) error) *Client {
	c := &Client{
		openURLFn:   defaultOpenURL,
		implements:  make(map[string]bool),
		elicitation: el,
		convClient:  conv,
	}
	if openURL != nil {
		c.openURLFn = openURL
	}
	return c
}

// SetConversationID assigns conversation id used for conv-scoped elicitation routing.
func (c *Client) SetConversationID(id string) { c.convID = id }

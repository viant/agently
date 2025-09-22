package mcp

// Client implements the mcp-protocol client-side Operations interface.  It is
// *not* a network client – instead it adapts protocol requests into local
// Agently capabilities (LLM generation, browser interaction, etc.).

import (
	"context"
	"fmt"

	awaitreg "github.com/viant/agently/genai/awaitreg"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/prompt"
	core2 "github.com/viant/agently/genai/service/core"

	"sync"

	elicitationSchema "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"

	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	elicitation "github.com/viant/agently/genai/elicitation"
	presetrefiner "github.com/viant/agently/genai/elicitation/refiner"
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
	core       *core2.Service
	openURLFn  func(string) error
	implements map[string]bool
	awaiters   *awaitreg.Registry
	llmCore    *core2.Service
	// waiter registries are shared for elicitation and interaction
	router interface {
		Register(uint64, string, chan *schema.ElicitResult)
		Remove(uint64, string)
	}
	convID     string
	onRefine   func(rs *schema.ElicitRequestParamsRequestedSchema)
	convClient apiconv.Client
	elicition  *elicitation.Service
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
	// Work on a copy and refine the schema for better UX prior to prompting.
	params := request.Request.Params
	if c.elicition != nil {
		c.elicition.RefineRequestedSchema(&params.RequestedSchema)
	} else if c.onRefine != nil {
		c.onRefine(&params.RequestedSchema)
	} else {
		// Fallback to preset for safety if service not initialised
		presetrefiner.Refine(&params.RequestedSchema)
	}
	// Persist & register typed-id waiter with optional router scoped by conversation id.
	ch := make(chan *schema.ElicitResult, 1)
	convID := c.convID
	if strings.TrimSpace(convID) == "" {
		convID = conversation.ID(ctx)
	}
	// Persist elicitation message so UIs can render it.
	if err := c.persistElicitationMessage(ctx, &params); err != nil {
		return nil, jsonrpc.NewInternalError(fmt.Sprintf("failed to persist elicitation: %v", err), nil)
	}
	// best-effort: register in conv-scoped router
	if c.router != nil {
		c.router.Register(request.Id, convID, ch)
		defer c.router.Remove(request.Id, convID)
	}
	// If awaiters configured (CLI), prompt in background and submit via channel
	if c.awaiters != nil {
		localReq := &elicitationSchema.Elicitation{ElicitRequestParams: params}
		if strings.TrimSpace(params.Url) != "" && c.openURLFn != nil {
			_ = c.openURLFn(params.Url)
		}
		aw := c.awaiters.Ensure(convID)
		go func() {
			res, err := aw.AwaitElicitation(ctx, localReq)
			var out *schema.ElicitResult
			if err != nil {
				out = &schema.ElicitResult{Action: schema.ElicitResultAction("decline")}
			} else {
				out = &schema.ElicitResult{Action: schema.ElicitResultAction(res.Action), Content: res.Payload}
			}
			select {
			case ch <- out:
			default:
			}
		}()
	}

	select {
	case res := <-ch:
		if res == nil {
			return nil, jsonrpc.NewInternalError("nil elicitation result", nil)
		}
		// Persist result and update status – bubble up any failure.
		if err := c.postElicitationResult(ctx, params.ElicitationId, res); err != nil {
			return nil, jsonrpc.NewInternalError(fmt.Sprintf("persist elicitation result: %v", err), nil)
		}
		if err := c.updateElicitationStatus(ctx, params.ElicitationId, string(res.Action)); err != nil {
			return nil, jsonrpc.NewInternalError(fmt.Sprintf("update elicitation status: %v", err), nil)
		}
		return res, nil
	case <-ctx.Done():
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
func (c *Client) persistElicitationMessage(ctx context.Context, params *schema.ElicitRequestParams) error {
	if strings.TrimSpace(c.convID) == "" {
		return fmt.Errorf("convID is required")
	}
	if strings.TrimSpace(params.ElicitationId) == "" {
		params.ElicitationId = uuid.New().String()
	}
	payload := &elicitationSchema.Elicitation{ElicitRequestParams: *params}
	// Record as tool-role elicitation (type=elicitation, status=pending), linked to current turn via context.
	if c.elicition != nil {
		return c.elicition.Record(ctx, c.convID, "tool", "", payload)
	}
	// Fallback (should not happen when convClient is set)
	return fmt.Errorf("elicitation service not initialized")
}

// postElicitationStatus emits a small status message linked to the elicitation
// so that Web/CLI UIs can visualise the resolution.
func (c *Client) postElicitationStatus(ctx context.Context, elicitationID, status string) error {
	if c.convClient == nil || strings.TrimSpace(c.convID) == "" || strings.TrimSpace(elicitationID) == "" {
		return fmt.Errorf("convClient/convID/elicitationID required")
	}
	// normalise to accepted/rejected/cancel
	st := strings.ToLower(strings.TrimSpace(status))
	switch st {
	case "accept", "accepted", "approve", "approved", "yes", "y":
		st = "accepted"
	case "decline", "declined", "reject", "rejected", "no", "n":
		st = "rejected"
	case "cancel", "canceled", "cancelled":
		st = "cancel"
	default:
		// leave as-is but lowercased
	}
	msg := apiconv.NewMessage()
	msg.SetId(uuid.New().String())
	msg.SetConversationID(c.convID)
	msg.SetElicitationID(elicitationID)
	// Try to link to the same turn as the original elicitation message
	if cv, err := c.convClient.GetConversation(ctx, c.convID); err == nil && cv != nil {
		for _, turn := range cv.GetTranscript() {
			if turn == nil {
				continue
			}
			for _, m := range turn.GetMessages() {
				if m == nil || m.ElicitationId == nil {
					continue
				}
				if strings.TrimSpace(*m.ElicitationId) == strings.TrimSpace(elicitationID) {
					if turnId := strings.TrimSpace(turn.Id); turnId != "" {
						msg.SetTurnID(turnId)
					}
					break
				}
			}
		}
	}
	msg.SetContent(st)
	if err := c.convClient.PatchMessage(ctx, msg); err != nil {
		return err
	}
	return nil
}

// postElicitationResult persists the resolved elicitation payload so that UIs
// and potential resume logic can consume it from storage.
func (c *Client) postElicitationResult(ctx context.Context, elicitationID string, res *schema.ElicitResult) error {
	if c.convClient == nil || strings.TrimSpace(c.convID) == "" || strings.TrimSpace(elicitationID) == "" || res == nil {
		return fmt.Errorf("convClient/convID/elicitationID/result required")
	}
	orig, err := c.convClient.GetMessageByElicitation(ctx, c.convID, elicitationID)
	if err != nil || orig == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("elicitation message not found")
	}
	role := strings.ToLower(strings.TrimSpace(orig.Role))
	if role == "assistant" {
		if c.elicition == nil {
			return fmt.Errorf("elicitation service not initialised")
		}
		return c.elicition.AddUserResponseMessage(ctx, c.convID, conv.Dereference[string](orig.TurnId), orig.Id, res.Content)
	}
	if c.elicition == nil {
		return fmt.Errorf("elicitation service not initialised")
	}
	return c.elicition.StoreToolResponse(ctx, c.convID, elicitationID, res.Content)
}

// updateElicitationStatus patches the original elicitation message status based on action.
func (c *Client) updateElicitationStatus(ctx context.Context, elicitationID, action string) error {
	if c.convClient == nil || strings.TrimSpace(c.convID) == "" || strings.TrimSpace(elicitationID) == "" {
		return fmt.Errorf("convClient/convID/elicitationID required")
	}
	st := strings.ToLower(strings.TrimSpace(action))
	switch st {
	case "accept", "accepted", "approve", "approved", "yes", "y":
		st = "accepted"
	case "decline", "declined", "reject", "rejected", "no", "n":
		st = "rejected"
	case "cancel", "canceled", "cancelled":
		st = "cancel"
	default:
		st = "rejected"
	}
	if c.elicition == nil {
		return fmt.Errorf("elicitation service not initialised")
	}
	return c.elicition.UpdateStatus(ctx, c.convID, elicitationID, action)
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

// SetConversationID assigns conversation id used for conv-scoped elicitation routing.
func (c *Client) SetConversationID(id string) { c.convID = id }

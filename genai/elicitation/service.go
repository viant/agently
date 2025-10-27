package elicitation

// moved from genai/service/elicitation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
	elact "github.com/viant/agently/genai/elicitation/action"
	elicrouter "github.com/viant/agently/genai/elicitation/router"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/mcp-protocol/schema"
)

type Refiner interface {
	RefineRequestedSchema(rs *schema.ElicitRequestParamsRequestedSchema)
}

type Service struct {
	client         apiconv.Client
	refiner        Refiner
	router         elicrouter.ElicitationRouter
	awaiterFactory func() Awaiter
}

// New constructs the elicitation service with all collaborators.
// The refiner is defaulted to a workspace preset implementation when nil.
// Router and awaiter factory must be supplied by the caller to ensure proper wiring.
func New(client apiconv.Client, refiner Refiner, router elicrouter.ElicitationRouter, awaiterFactory func() Awaiter) *Service {
	if refiner == nil {
		refiner = DefaultRefiner{}
	}
	return &Service{client: client, refiner: refiner, router: router, awaiterFactory: awaiterFactory}
}

func (s *Service) RefineRequestedSchema(rs *schema.ElicitRequestParamsRequestedSchema) {
	if rs == nil {
		return
	}
	if s != nil && s.refiner != nil {
		s.refiner.RefineRequestedSchema(rs)
		return
	}
	DefaultRefiner{}.RefineRequestedSchema(rs)
}

// Record persists an elicitation control message and returns its message id.
func (s *Service) Record(ctx context.Context, turn *memory.TurnMeta, role string, elic *plan.Elicitation) (*apiconv.MutableMessage, error) {
	if strings.TrimSpace(elic.ElicitationId) == "" {
		elic.ElicitationId = uuid.New().String()
	}
	s.RefineRequestedSchema(&elic.RequestedSchema)
	// Provide a unified callback URL when not already set
	if strings.TrimSpace(elic.CallbackURL) == "" && turn != nil {
		elic.CallbackURL = fmt.Sprintf("/v1/api/conversations/%s/elicitation/%s", turn.ConversationID, elic.ElicitationId)
	}
	raw, _ := json.Marshal(elic)
	messageType := "control"
	if role == llm.RoleAssistant.String() {
		messageType = "text"
	}
	msg, err := apiconv.AddMessage(ctx, s.client, turn,
		apiconv.WithId(uuid.New().String()),
		apiconv.WithRole(role),
		apiconv.WithType(messageType),
		apiconv.WithElicitationID(elic.ElicitationId),
		apiconv.WithStatus("pending"),
		apiconv.WithContent(string(raw)),
	)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// Wait blocks until an elicitation is accepted/declined via router/UI or optional local awaiter.
// On accept, it best-effort persists payload and status. It returns (accepted, payload, error).
func (s *Service) Wait(ctx context.Context, convID, elicitationID string) (string, map[string]interface{}, error) {
	if s.router == nil {
		return "", nil, fmt.Errorf("elicitation router not configured")
	}
	if strings.TrimSpace(convID) == "" || strings.TrimSpace(elicitationID) == "" {
		return "", nil, fmt.Errorf("conversation and elicitation id required")
	}
	ch := make(chan *schema.ElicitResult, 1)
	s.router.RegisterByElicitationID(convID, elicitationID, ch)
	defer s.router.RemoveByElicitation(convID, elicitationID)

	// Spawn local awaiter if configured. Retrieve original elicitation schema to prompt properly.
	if s.awaiterFactory != nil {
		go func() {
			var req plan.Elicitation
			if msg, err := s.client.GetMessageByElicitation(ctx, convID, elicitationID); err == nil && msg != nil && msg.Content != nil {
				if c := strings.TrimSpace(*msg.Content); c != "" {
					_ = json.Unmarshal([]byte(c), &req)
				}
			}
			// Ensure ElicitationId is present
			req.ElicitRequestParams.ElicitationId = elicitationID
			aw := s.awaiterFactory()
			res, err := aw.AwaitElicitation(ctx, &req)
			if err != nil || res == nil {
				return
			}
			// Persist when accepted and notify router
			action := strings.ToLower(string(res.Action))
			switch action {
			case elact.Accept:
				if res.Payload != nil {
					_ = s.StorePayload(ctx, convID, elicitationID, res.Payload)
					_ = s.UpdateStatus(ctx, convID, elicitationID, elact.Accept)
				}
				// If accepted without payload, do not mark declined; UI callback should resolve.
			default: // decline or other
				_ = s.UpdateStatus(ctx, convID, elicitationID, elact.Decline)
				if strings.TrimSpace(res.Reason) != "" {
					_ = s.StoreDeclineReason(ctx, convID, elicitationID, res.Reason)
				}
			}
			out := &schema.ElicitResult{Action: schema.ElicitResultAction(elact.Normalize(string(res.Action))), Content: res.Payload}
			s.router.AcceptByElicitation(convID, elicitationID, out)
		}()
	}

	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case res := <-ch:
		if res == nil {
			return elact.Decline, nil, nil
		}
		act := elact.Normalize(string(res.Action))
		return act, res.Content, nil
	}
}

// Elicit records a new elicitation control message and waits for a resolution via router/UI.
// Returns message id, normalized status (accepted/rejected/cancel) and optional payload.
func (s *Service) Elicit(ctx context.Context, turn *memory.TurnMeta, role string, req *plan.Elicitation) (string, string, map[string]interface{}, error) {
	if req == nil || turn == nil {
		return "", "", nil, fmt.Errorf("invalid input")
	}

	msg, err := s.Record(ctx, turn, role, req)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to record message: %w", err)
	}
	root := s.getRootConversation(ctx, turn.ConversationID)
	// Only duplicate into a different conversation. If getRootConversation returns
	// the same conversation (e.g. when at the root or due to lookup quirks), skip.
	if root != nil && strings.TrimSpace(root.Id) != "" && root.Id != turn.ConversationID {
		rootConversationMessage := *msg
		rootConversationMessage.SetId(uuid.New().String())
		if root.LastTurnId != nil {
			rootConversationMessage.SetTurnID(*root.LastTurnId)
			rootConversationMessage.SetConversationID(root.Id)
		}
		if err := s.client.PatchMessage(ctx, &rootConversationMessage); err != nil {
			return "", "", nil, fmt.Errorf("failed to root record message: %w", err)
		}

		cloneMsg := apiconv.NewMessage()
		cloneMsg.SetId(msg.Id)
		cloneMsg.SetParentMessageID(rootConversationMessage.Id)
		_ = s.client.PatchMessage(ctx, cloneMsg)
	}

	status, payload, err := s.Wait(ctx, turn.ConversationID, req.ElicitationId)
	if err != nil {
		return msg.Id, "", nil, err
	}
	return msg.Id, status, payload, nil
}

func (s *Service) getRootConversation(ctx context.Context, conversationId string) *apiconv.Conversation {
	var conv *apiconv.Conversation
	if parent, err := s.client.GetConversation(ctx, conversationId); err == nil && parent != nil {
		if parent.ConversationParentId != nil {
			conv = parent
			if ret := s.getRootConversation(ctx, *conv.ConversationParentId); ret != nil {
				return ret
			}
		}
	}
	return conv
}

func (s *Service) UpdateStatus(ctx context.Context, convID, elicitationID, action string) error {
	st := elact.ToStatus(action)
	msg, err := s.client.GetMessageByElicitation(ctx, convID, elicitationID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("elicitation message not found")
	}
	upd := apiconv.NewMessage()
	upd.SetId(msg.Id)
	upd.SetStatus(st)
	if err := s.client.PatchMessage(ctx, upd); err != nil {
		return err
	}
	// Log status update for visibility (tool or assistant)
	role := strings.TrimSpace(msg.Role)
	log.Printf("[elicitation] status update: conv=%s elic=%s msg=%s role=%s status=%s", convID, elicitationID, msg.Id, role, st)
	if msg.ParentMessageId != nil {
		if dep, err := s.client.GetMessage(ctx, *msg.ParentMessageId); err == nil && dep != nil {
			return s.client.DeleteMessage(ctx, dep.ConversationId, dep.Id)
		}
	}
	return nil
}

func (s *Service) StorePayload(ctx context.Context, convID, elicitationID string, payload map[string]interface{}) error {
	msg, err := s.client.GetMessageByElicitation(ctx, convID, elicitationID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("elicitation message not found")
	}
	raw, _ := json.Marshal(payload)
	pid := uuid.New().String()
	p := apiconv.NewPayload()
	p.SetId(pid)
	p.SetKind("elicitation_response")
	p.SetMimeType("application/json")
	p.SetSizeBytes(len(raw))
	p.SetStorage("inline")
	p.SetInlineBody(raw)
	if err := s.client.PatchPayload(ctx, p); err != nil {
		return err
	}
	upd := apiconv.NewMessage()
	upd.SetId(msg.Id)
	upd.SetElicitationPayloadID(pid)
	if msg.Role == llm.RoleAssistant.String() {
		turn := memory.TurnMeta{TurnID: *msg.TurnId, ConversationID: msg.ConversationId, ParentMessageID: *msg.ParentMessageId}
		if err := s.AddUserResponseMessage(ctx, &turn, payload); err != nil {
			return err
		}
	}
	log.Printf("[elicitation] payload stored: conv=%s elic=%s msg=%s bytes=%d", convID, elicitationID, msg.Id, len(raw))
	return s.client.PatchMessage(ctx, upd)
}

func (s *Service) AddUserResponseMessage(ctx context.Context, turn *memory.TurnMeta, payload map[string]interface{}) error {
	raw, _ := json.Marshal(payload)
	_, err := apiconv.AddMessage(ctx, s.client, turn,
		apiconv.WithId(uuid.New().String()),
		apiconv.WithRole("user"),
		apiconv.WithType("text"),
		apiconv.WithContent(string(raw)),
	)
	return err
}

// NormalizeAction is kept for backward compatibility; use action.Normalize.
func NormalizeAction(a string) string { return elact.Normalize(a) }

// HandleCallback processes an elicitation decision end-to-end:
// - normalizes the action
// - updates message status
// - stores payload (when accepted)
// - notifies any registered router waiter
func (s *Service) HandleCallback(ctx context.Context, convID, elicitationID, action string, payload map[string]interface{}) error {
	// Deprecated: prefer Resolve
	return s.Resolve(ctx, convID, elicitationID, action, payload, "")
}

// Resolve processes an elicitation decision end-to-end:
// - normalizes the action
// - updates message status
// - stores payload (when accepted)
// - notifies any registered router waiter
func (s *Service) Resolve(ctx context.Context, convID, elicitationID, action string, payload map[string]interface{}, reason string) error {
	if strings.TrimSpace(convID) == "" || strings.TrimSpace(elicitationID) == "" {
		return fmt.Errorf("conversation and elicitation id required")
	}
	act := elact.Normalize(action)
	// Log acceptance/decline with minimal context
	hasPayload := payload != nil && len(payload) > 0
	log.Printf("[elicitation] resolve: conv=%s elic=%s action=%s payload=%v", convID, elicitationID, act, hasPayload)
	if err := s.UpdateStatus(ctx, convID, elicitationID, act); err != nil {
		return err
	}
	if elact.ToStatus(act) == elact.StatusAccepted && payload != nil {
		if err := s.StorePayload(ctx, convID, elicitationID, payload); err != nil {
			return err
		}
	} else if elact.ToStatus(act) == elact.StatusRejected && strings.TrimSpace(reason) != "" {
		if err := s.StoreDeclineReason(ctx, convID, elicitationID, reason); err != nil {
			return err
		}
	}
	out := &schema.ElicitResult{Action: schema.ElicitResultAction(act), Content: payload}
	s.router.AcceptByElicitation(convID, elicitationID, out)
	return nil
}

// StoreDeclineReason persists a user-decline reason as a user message so the agent can react.
func (s *Service) StoreDeclineReason(ctx context.Context, convID, elicitationID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return nil
	}
	msg, err := s.client.GetMessageByElicitation(ctx, convID, elicitationID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("elicitation message not found")
	}
	// Only add a user response message when the elicitation originated from an assistant message
	if msg.Role != llm.RoleAssistant.String() {
		return nil
	}
	turn := memory.TurnMeta{TurnID: *msg.TurnId, ConversationID: msg.ConversationId, ParentMessageID: *msg.ParentMessageId}
	payload := map[string]interface{}{"declineReason": reason}
	return s.AddUserResponseMessage(ctx, &turn, payload)
}

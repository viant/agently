package elicitation

// moved from genai/service/elicitation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
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
func (s *Service) Record(ctx context.Context, turn *memory.TurnMeta, role string, elic *plan.Elicitation) (string, error) {
	if strings.TrimSpace(elic.ElicitationId) == "" {
		elic.ElicitationId = uuid.New().String()
	}
	s.RefineRequestedSchema(&elic.RequestedSchema)
	// Provide a unified callback URL when not already set
	if strings.TrimSpace(elic.CallbackURL) == "" && turn != nil {
		elic.CallbackURL = fmt.Sprintf("/v1/api/conversations/%s/elicitation/%s", turn.ConversationID, elic.ElicitationId)
	}
	raw, _ := json.Marshal(elic)
	msg := apiconv.NewMessage()
	msg.SetId(uuid.New().String())
	msg.SetConversationID(turn.ConversationID)
	if strings.TrimSpace(turn.TurnID) != "" {
		msg.SetTurnID(turn.TurnID)
	}
	msg.SetElicitationID(elic.ElicitationId)
	if msg.TurnID == nil {
		if cv, err := s.client.GetConversation(ctx, turn.ConversationID); err == nil && cv != nil && cv.LastTurnId != nil && strings.TrimSpace(*cv.LastTurnId) != "" {
			msg.SetTurnID(*cv.LastTurnId)
		}
	}
	if strings.TrimSpace(turn.ParentMessageID) != "" {
		msg.SetParentMessageID(turn.ParentMessageID)
	}

	msg.SetRole(role)
	messageType := "control"
	if role == llm.RoleAssistant.String() {
		messageType = "text"
	}

	msg.SetType(messageType)
	msg.Status = "pending"
	if msg.Has != nil {
		msg.Has.Status = true
	}
	if len(raw) > 0 {
		msg.SetContent(string(raw))
	}
	if err := s.client.PatchMessage(ctx, msg); err != nil {
		return "", err
	}
	return msg.Id, nil
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
			if strings.ToLower(string(res.Action)) == "accept" && res.Payload != nil {
				_ = s.StorePayload(ctx, convID, elicitationID, res.Payload)
				_ = s.UpdateStatus(ctx, convID, elicitationID, "accepted")
			} else {
				_ = s.UpdateStatus(ctx, convID, elicitationID, "rejected")
			}
			out := &schema.ElicitResult{Action: schema.ElicitResultAction(res.Action), Content: res.Payload}
			s.router.AcceptByElicitation(convID, elicitationID, out)
		}()
	}

	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case res := <-ch:
		if res == nil {
			return "rejected", nil, nil
		}
		st := NormalizeAction(string(res.Action))
		return st, res.Content, nil
	}
}

// Elicit records a new elicitation control message and waits for a resolution via router/UI.
// Returns message id, normalized status (accepted/rejected/cancel) and optional payload.
func (s *Service) Elicit(ctx context.Context, turn *memory.TurnMeta, role string, req *plan.Elicitation) (string, string, map[string]interface{}, error) {
	if req == nil || turn == nil {
		return "", "", nil, fmt.Errorf("invalid input")
	}
	msgID, err := s.Record(ctx, turn, role, req)
	if err != nil {
		return "", "", nil, err
	}
	status, payload, err := s.Wait(ctx, turn.ConversationID, req.ElicitationId)
	if err != nil {
		return msgID, "", nil, err
	}
	return msgID, status, payload, nil
}

func (s *Service) UpdateStatus(ctx context.Context, convID, elicitationID, action string) error {
	st := NormalizeAction(action)
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
	return s.client.PatchMessage(ctx, upd)
}

func (s *Service) StorePayload(ctx context.Context, convID, elicitationID string, payload map[string]interface{}) error {
	return StorePayload(ctx, s.client, convID, elicitationID, payload)
}

func (s *Service) AddUserResponseMessage(ctx context.Context, tm *memory.TurnMeta, payload map[string]interface{}) error {
	raw, _ := json.Marshal(payload)
	m := apiconv.NewMessage()
	m.SetId(uuid.New().String())
	m.SetConversationID(tm.ConversationID)
	if strings.TrimSpace(tm.TurnID) != "" {
		m.SetTurnID(tm.TurnID)
	}
	if strings.TrimSpace(tm.ParentMessageID) != "" {
		m.SetParentMessageID(tm.ParentMessageID)
	}
	m.SetRole("user")
	m.SetType("text")
	m.SetContent(string(raw))
	return s.client.PatchMessage(ctx, m)
}

func NormalizeAction(action string) string {
	st := strings.ToLower(strings.TrimSpace(action))
	switch st {
	case "accept", "accepted", "approve", "approved", "yes", "y":
		return "accepted"
	case "cancel", "canceled", "cancelled":
		return "cancel"
	case "decline", "declined", "reject", "rejected", "no", "n":
		fallthrough
	default:
		return "rejected"
	}
}

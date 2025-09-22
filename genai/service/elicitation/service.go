package elicitation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent/plan"
	presetrefiner "github.com/viant/agently/genai/io/elicitation/refiner"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/internal/conv"
	"github.com/viant/mcp-protocol/schema"
)

// Refiner is a minimal interface for schema refinement.
type Refiner interface {
	RefineRequestedSchema(rs *schema.ElicitRequestParamsRequestedSchema)
}

type Service struct {
	client  apiconv.Client
	refiner Refiner
}

type Option func(*Service)

func WithRefiner(r Refiner) Option { return func(s *Service) { s.refiner = r } }

func New(client apiconv.Client, opts ...Option) *Service {
	s := &Service{client: client}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// SetRefiner allows late injection of a refiner.
func (s *Service) SetRefiner(r Refiner) { s.refiner = r }

// NormalizeAction maps various inputs to accepted/rejected/cancel.
func (Service) NormalizeAction(action string) string {
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

// Record creates an elicitation message with type=elicitation and status=pending.
// role should typically be "assistant" (LLM) or "tool".
func (s *Service) Record(ctx context.Context, convID, role, parentMessageID string, elic *plan.Elicitation) error {
	if s.client == nil || strings.TrimSpace(convID) == "" || elic == nil {
		return fmt.Errorf("invalid input")
	}
	// Refine schema for better UX.
	if s.refiner != nil {
		s.refiner.RefineRequestedSchema(&elic.RequestedSchema)
	} else {
		presetrefiner.Refine(&elic.RequestedSchema)
	}
	if strings.TrimSpace(elic.ElicitationId) == "" {
		elic.ElicitationId = uuid.New().String()
	}
	raw, _ := json.Marshal(elic)
	msg := apiconv.NewMessage()
	msg.SetId(uuid.New().String())
	msg.SetConversationID(convID)
	if tm, ok := memory.TurnMetaFromContext(ctx); ok && strings.TrimSpace(tm.TurnID) != "" {
		msg.SetTurnID(tm.TurnID)
	}
	msg.SetElicitationID(elic.ElicitationId)
	if strings.TrimSpace(parentMessageID) != "" {
		msg.SetParentMessageID(parentMessageID)
	}
	msg.SetRole(role)
	msg.SetType("elicitation")
	msg.Status = "pending"
	if msg.Has != nil {
		msg.Has.Status = true
	}
	if len(raw) > 0 {
		msg.SetContent(string(raw))
	}
	return s.client.PatchMessage(ctx, msg)
}

// UpdateStatus updates the original elicitation message status in place.
func (s *Service) UpdateStatus(ctx context.Context, convID, elicitationID, action string) error {
	if s.client == nil || strings.TrimSpace(convID) == "" || strings.TrimSpace(elicitationID) == "" {
		return fmt.Errorf("invalid input")
	}
	st := s.NormalizeAction(action)
	msg, err := s.client.GetMessageByElicitation(ctx, convID, elicitationID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("elicitation message not found")
	}
	upd := apiconv.NewMessage()
	upd.SetId(msg.Id)
	upd.Status = st
	if upd.Has != nil {
		upd.Has.Status = true
	}
	return s.client.PatchMessage(ctx, upd)
}

// StoreToolResponse stores the payload as an inline call_payload and links it to the original message.
func (s *Service) StoreToolResponse(ctx context.Context, convID, elicitationID string, payload map[string]interface{}) error {
	if s.client == nil || strings.TrimSpace(convID) == "" || strings.TrimSpace(elicitationID) == "" {
		return fmt.Errorf("invalid input")
	}
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
	upd.SetPayloadID(pid)
	return s.client.PatchMessage(ctx, upd)
}

// AddUserResponseMessage adds a user text message with the JSON payload for LLM history.
func (s *Service) AddUserResponseMessage(ctx context.Context, convID, turnID, parentMessageID string, payload map[string]interface{}) error {
	if s.client == nil || strings.TrimSpace(convID) == "" {
		return fmt.Errorf("invalid input")
	}
	raw, _ := json.Marshal(payload)
	m := apiconv.NewMessage()
	m.SetId(uuid.New().String())
	m.SetConversationID(convID)
	if strings.TrimSpace(turnID) != "" {
		m.SetTurnID(turnID)
	}
	if strings.TrimSpace(parentMessageID) != "" {
		m.SetParentMessageID(parentMessageID)
	}
	m.SetRole("user")
	m.SetType("text")
	m.SetContent(string(raw))
	return s.client.PatchMessage(ctx, m)
}

// Deref is a small helper to dereference optional fields.
func Deref(s *string) string { return conv.Dereference[string](s) }

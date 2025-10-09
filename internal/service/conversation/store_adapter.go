package conversation

import (
	"context"
	"strings"

	chat "github.com/viant/agently/client/chat"
	chstore "github.com/viant/agently/client/chat/store"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/datly"
)

// StoreAdapter implements chat/store.Client on top of the existing Service.
type StoreAdapter struct{ svc *Service }

func NewStoreAdapter(svc *Service) chstore.Client { return &StoreAdapter{svc: svc} }

func (s *StoreAdapter) GetConversations(ctx context.Context) (*agconv.ConversationOutput, error) {
	if s == nil || s.svc == nil || s.svc.dao == nil {
		return nil, nil
	}
	in := agconv.ConversationInput{}
	out := &agconv.ConversationOutput{}
	if _, err := s.svc.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(agconv.ConversationsPathURI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *StoreAdapter) GetConversation(ctx context.Context, id string, options ...chat.Option) (*agconv.ConversationView, error) {
	if s == nil || s.svc == nil || s.svc.dao == nil {
		return nil, nil
	}
	inSDK := chat.Input{Id: id, Has: &agconv.ConversationInputHas{Id: true}}
	for _, opt := range options {
		if opt != nil {
			opt(&inSDK)
		}
	}
	in := agconv.ConversationInput(inSDK)
	out := &agconv.ConversationOutput{}
	uri := strings.ReplaceAll(agconv.ConversationPathURI, "{id}", id)
	if _, err := s.svc.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	return out.Data[0], nil
}

// The remaining methods delegate to the underlying Service which already implements them.
func (s *StoreAdapter) PatchConversations(ctx context.Context, conv *chat.MutableConversation) error {
	return s.svc.PatchConversations(ctx, conv)
}
func (s *StoreAdapter) GetPayload(ctx context.Context, id string) (*chat.Payload, error) {
	return s.svc.GetPayload(ctx, id)
}
func (s *StoreAdapter) PatchPayload(ctx context.Context, payload *chat.MutablePayload) error {
	return s.svc.PatchPayload(ctx, payload)
}
func (s *StoreAdapter) PatchMessage(ctx context.Context, message *chat.MutableMessage) error {
	return s.svc.PatchMessage(ctx, message)
}
func (s *StoreAdapter) GetMessage(ctx context.Context, id string) (*chat.Message, error) {
	return s.svc.GetMessage(ctx, id)
}
func (s *StoreAdapter) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*chat.Message, error) {
	return s.svc.GetMessageByElicitation(ctx, conversationID, elicitationID)
}
func (s *StoreAdapter) PatchModelCall(ctx context.Context, mc *chat.MutableModelCall) error {
	return s.svc.PatchModelCall(ctx, mc)
}
func (s *StoreAdapter) PatchToolCall(ctx context.Context, tc *chat.MutableToolCall) error {
	return s.svc.PatchToolCall(ctx, tc)
}
func (s *StoreAdapter) PatchTurn(ctx context.Context, turn *chat.MutableTurn) error {
	return s.svc.PatchTurn(ctx, turn)
}
func (s *StoreAdapter) DeleteConversation(ctx context.Context, id string) error {
	return s.svc.DeleteConversation(ctx, id)
}

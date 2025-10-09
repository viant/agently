package conversation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unsafe"

	chat "github.com/viant/agently/client/chat"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convdel "github.com/viant/agently/pkg/agently/conversation/delete"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	messageread "github.com/viant/agently/pkg/agently/message/read"
	msgwrite "github.com/viant/agently/pkg/agently/message/write"
	modelcallwrite "github.com/viant/agently/pkg/agently/modelcall/write"
	payloadread "github.com/viant/agently/pkg/agently/payload/read"
	payloadwrite "github.com/viant/agently/pkg/agently/payload/write"
	toolcallwrite "github.com/viant/agently/pkg/agently/toolcall/write"
	turnwrite "github.com/viant/agently/pkg/agently/turn/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

// StoreAdapter implements chstore.Client; see store_adapter.go.

// New constructs a conversation Service using the provided datly service
// and registers the rich conversation components.
func New(ctx context.Context, dao *datly.Service) (*Service, error) {
	if dao == nil {
		return nil, nil
	}
	srv := &Service{dao: dao}
	err := srv.init(ctx, dao)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

func (s *Service) init(ctx context.Context, dao *datly.Service) error {
	if err := agconv.DefineConversationComponent(ctx, dao); err != nil {
		return err
	}
	if err := agconv.DefineConversationsComponent(ctx, dao); err != nil {
		return err
	}
	if err := messageread.DefineMessageComponent(ctx, dao); err != nil {
		return err
	}
	if err := messageread.DefineMessageByElicitationComponent(ctx, dao); err != nil {
		return err
	}
	if err := payloadread.DefineComponent(ctx, dao); err != nil {
		return err
	}

	if _, err := convw.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := convw.DefinePostComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := msgwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := modelcallwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := toolcallwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := turnwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := payloadwrite.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := convdel.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

func (s *Service) PatchConversations(ctx context.Context, conversations *chat.MutableConversation) error {
	conv := []*convw.Conversation{(*convw.Conversation)(conversations)}
	input := &convw.Input{Conversations: conv}
	out := &convw.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, convw.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

// GetConversations implements conversation.API using the generated component and returns SDK Conversation.
func (s *Service) GetConversations(ctx context.Context) ([]*chat.Conversation, error) {
	// Default: filter to non-scheduled conversations only via Scheduled=0
	val := 0
	inSDK := chat.Input{IncludeTranscript: false, Scheduled: val, Has: &agconv.ConversationInputHas{IncludeTranscript: true, Scheduled: true}}
	// Map SDK input to generated input
	in := agconv.ConversationInput(inSDK)
	out := &agconv.ConversationOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(agconv.ConversationsPathURI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	result := *(*[]*chat.Conversation)(unsafe.Pointer(&out.Data))
	return result, nil
}

// GetConversation implements conversation.API using the generated component and returns SDK Conversation.
func (s *Service) GetConversation(ctx context.Context, id string, options ...chat.Option) (*chat.Conversation, error) {
	// Build SDK input via options
	inSDK := chat.Input{Id: id, Has: &agconv.ConversationInputHas{Id: true}}
	for _, opt := range options {
		if opt != nil {
			opt(&inSDK)
		}
	}
	// Map SDK input to generated input
	in := agconv.ConversationInput(inSDK)

	out := &agconv.ConversationOutput{}
	uri := strings.ReplaceAll(agconv.ConversationPathURI, "{id}", id)
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
		return nil, err
	}

	if len(out.Data) == 0 {
		// No conversation found; mirror API behavior by returning nil without logging.
		return nil, nil
	}
	// Cast generated to SDK type
	conv := chat.Conversation(*out.Data[0])
	return &conv, nil
}

func (s *Service) GetPayload(ctx context.Context, id string) (*chat.Payload, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	in := payloadread.Input{Id: id, Has: &payloadread.Has{Id: true}}
	out := &payloadread.Output{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(payloadread.PayloadURI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	res := chat.Payload(*out.Data[0])
	return &res, nil
}

func (s *Service) PatchPayload(ctx context.Context, payload *chat.MutablePayload) error {
	if s == nil || s.dao == nil || payload == nil {
		return nil
	}
	// MutablePayload is an alias of pkg/agently/payload.Payload
	pw := (*payloadwrite.Payload)(payload)
	input := &payloadwrite.Input{Payloads: []*payloadwrite.Payload{pw}}
	out := &payloadwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, payloadwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) GetMessage(ctx context.Context, id string) (*chat.Message, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	in := messageread.MessageInput{Id: id, Has: &messageread.MessageInputHas{Id: true}}
	out := &messageread.MessageOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(messageread.MessagePathURI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	res := chat.Message(*out.Data[0])
	return &res, nil
}

func (s *Service) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*chat.Message, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	in := messageread.MessageByElicitationInput{ConversationId: conversationID, ElicitationId: elicitationID}
	out := &messageread.MessageByElicitationOutput{}
	uri := messageread.MessageByElicitationPathURI
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	res := chat.Message(*out.Data[0])
	return &res, nil
}

func (s *Service) PatchMessage(ctx context.Context, message *chat.MutableMessage) error {
	mm := (*msgwrite.Message)(message)
	input := &msgwrite.Input{Messages: []*msgwrite.Message{mm}}
	out := &msgwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, msgwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) PatchModelCall(ctx context.Context, modelCall *chat.MutableModelCall) error {
	if s == nil || s.dao == nil || modelCall == nil {
		return nil
	}
	mc := (*modelcallwrite.ModelCall)(modelCall)
	input := &modelcallwrite.Input{ModelCalls: []*modelcallwrite.ModelCall{mc}}
	out := &modelcallwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, modelcallwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) PatchToolCall(ctx context.Context, toolCall *chat.MutableToolCall) error {
	if s == nil || s.dao == nil || toolCall == nil {
		return nil
	}
	tc := (*toolcallwrite.ToolCall)(toolCall)
	input := &toolcallwrite.Input{ToolCalls: []*toolcallwrite.ToolCall{tc}}
	out := &toolcallwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, toolcallwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

func (s *Service) PatchTurn(ctx context.Context, turn *chat.MutableTurn) error {
	if s == nil || s.dao == nil || turn == nil {
		return nil
	}
	tr := (*turnwrite.Turn)(turn)
	input := &turnwrite.Input{Turns: []*turnwrite.Turn{tr}}
	out := &turnwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, turnwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

// DeleteConversation removes a conversation by id. Dependent rows are removed via DB FKs (ON DELETE CASCADE).
func (s *Service) DeleteConversation(ctx context.Context, id string) error {
	if s == nil || s.dao == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	in := &convdel.Input{Ids: []string{id}}
	out := &convdel.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodDelete, convdel.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return err
	}
	if len(out.Violations) > 0 {
		return errors.New(out.Violations[0].Message)
	}
	return nil
}

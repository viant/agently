package conversation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unsafe"

	convcli "github.com/viant/agently/client/conversation"
	agconv "github.com/viant/agently/pkg/agently/conversation"
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
	return nil
}

func (s *Service) PatchConversations(ctx context.Context, conversations *convcli.MutableConversation) error {
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
func (s *Service) GetConversations(ctx context.Context) ([]*convcli.Conversation, error) {
	inSDK := convcli.Input{IncludeTranscript: false, Has: &agconv.ConversationInputHas{IncludeTranscript: true}}
	// Map SDK input to generated input
	in := agconv.ConversationInput(inSDK)
	out := &agconv.ConversationOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(agconv.ConversationsPathURI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	result := *(*[]*convcli.Conversation)(unsafe.Pointer(&out.Data))
	return result, nil
}

// GetConversation implements conversation.API using the generated component and returns SDK Conversation.
func (s *Service) GetConversation(ctx context.Context, id string, options ...convcli.Option) (*convcli.Conversation, error) {
	// Build SDK input via options
	inSDK := convcli.Input{Id: id, Has: &agconv.ConversationInputHas{Id: true}}
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
		return nil, nil
	}
	// Cast generated to SDK type
	conv := convcli.Conversation(*out.Data[0])
	return &conv, nil
}

func (s *Service) GetPayload(ctx context.Context, id string) (*convcli.Payload, error) {
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
	res := convcli.Payload(*out.Data[0])
	return &res, nil
}

func (s *Service) PatchPayload(ctx context.Context, payload *convcli.MutablePayload) error {
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

func (s *Service) GetMessage(ctx context.Context, id string) (*convcli.Message, error) {
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
	res := convcli.Message(*out.Data[0])
	return &res, nil
}

func (s *Service) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*convcli.Message, error) {
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
	res := convcli.Message(*out.Data[0])
	return &res, nil
}

func (s *Service) PatchMessage(ctx context.Context, message *convcli.MutableMessage) error {
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

func (s *Service) PatchModelCall(ctx context.Context, modelCall *convcli.MutableModelCall) error {
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

func (s *Service) PatchToolCall(ctx context.Context, toolCall *convcli.MutableToolCall) error {
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

func (s *Service) PatchTurn(ctx context.Context, turn *convcli.MutableTurn) error {
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

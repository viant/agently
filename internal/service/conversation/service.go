package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"unsafe"

	convcli "github.com/viant/agently/client/conversation"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convdel "github.com/viant/agently/pkg/agently/conversation/delete"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgdel "github.com/viant/agently/pkg/agently/message/delete"
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
		return nil, errors.New("conversation service requires a non-nil datly.Service")
	}
	srv := &Service{dao: dao}
	err := srv.init(ctx, dao)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

func (s *Service) init(ctx context.Context, dao *datly.Service) error {
	var initErr error
	componentsOnce.Do(func() {
		if err := agconv.DefineConversationComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := agconv.DefineConversationsComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := messageread.DefineMessageComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := messageread.DefineMessageByElicitationComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := payloadread.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}

		if _, err := convw.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := msgwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := modelcallwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := toolcallwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := turnwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := payloadwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := convdel.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := msgdel.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
	})
	return initErr
}

var componentsOnce sync.Once

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
func (s *Service) GetConversations(ctx context.Context, input *convcli.Input) ([]*convcli.Conversation, error) {
	// Default: filter to non-scheduled conversations only via Scheduled=0
	if input.Has == nil {
		input.Has = &agconv.ConversationInputHas{}
	}
	input.IncludeTranscript = true
	input.Has.IncludeTranscript = true
	out := &agconv.ConversationOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(agconv.ConversationsPathURI), datly.WithInput(input)); err != nil {
		return nil, err
	}
	i, _ := json.Marshal(input)
	m, _ := json.Marshal(out.Metrics)
	d, _ := json.Marshal(out.Data)
	fmt.Printf("Input: %s, Metrics: %s\nData: %v\n", i, m, len(d))

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
		// No conversation found; mirror API behavior by returning nil without logging.
		return nil, nil
	}
	// Cast generated to SDK type
	conv := convcli.Conversation(*out.Data[0])
	return &conv, nil
}

func (s *Service) GetPayload(ctx context.Context, id string) (*convcli.Payload, error) {
	if s == nil || s.dao == nil {
		return nil, errors.New("conversation service not configured: dao is nil")
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
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if payload == nil {
		return errors.New("invalid payload: nil")
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

func (s *Service) GetMessage(ctx context.Context, id string, options ...convcli.Option) (*convcli.Message, error) {
	if s == nil || s.dao == nil {
		return nil, errors.New("conversation service not configured: dao is nil")
	}
	// Map conversation-style options to message read input flags
	var convIn convcli.Input
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(&convIn)
	}
	in := messageread.MessageInput{Id: id, Has: &messageread.MessageInputHas{Id: true}}
	if convIn.Has != nil {
		if convIn.Has.IncludeToolCall && convIn.IncludeToolCall {
			in.IncludeToolCall = true
			in.Has.IncludeToolCall = true
		}
		if convIn.Has.IncludeModelCal && convIn.IncludeModelCal {
			in.IncludeModelCal = true
			in.Has.IncludeModelCal = true
		}
	}

	uri := strings.ReplaceAll(messageread.MessagePathURI, "{id}", id)
	out := &messageread.MessageOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
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
		return nil, errors.New("conversation service not configured: dao is nil")
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
		// Augment DB/validation error with key message fields to aid diagnosis
		return fmt.Errorf(
			"patch message failed (id=%s convo=%s turn=%v role=%s type=%s status=%q): %w",
			message.Id,
			message.ConversationID,
			valueOrEmpty(message.TurnID),
			strings.TrimSpace(message.Role),
			strings.TrimSpace(message.Type),
			strings.TrimSpace(message.Status),
			err,
		)
	}
	if len(out.Violations) > 0 {
		return fmt.Errorf(
			"patch message violation (id=%s convo=%s turn=%v role=%s type=%s status=%q): %s",
			message.Id,
			message.ConversationID,
			valueOrEmpty(message.TurnID),
			strings.TrimSpace(message.Role),
			strings.TrimSpace(message.Type),
			strings.TrimSpace(message.Status),
			out.Violations[0].Message,
		)
	}
	return nil
}

// valueOrEmpty renders pointer values without exposing nil dereference in logs.
func valueOrEmpty[T any](p *T) interface{} {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Service) PatchModelCall(ctx context.Context, modelCall *convcli.MutableModelCall) error {
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if modelCall == nil {
		return errors.New("invalid modelCall: nil")
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
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if toolCall == nil {
		return errors.New("invalid toolCall: nil")
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
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if turn == nil {
		return errors.New("invalid turn: nil")
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
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("conversation id is required")
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

// DeleteMessage removes a single message from a conversation using the dedicated DELETE component.
func (s *Service) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if strings.TrimSpace(messageID) == "" {
		return errors.New("message id is required")
	}

	// Optional safety check: if conversationID provided, verify the message belongs to it.
	if strings.TrimSpace(conversationID) != "" {
		if got, _ := s.GetMessage(ctx, messageID); got != nil && strings.TrimSpace(got.ConversationId) != "" {
			if !strings.EqualFold(strings.TrimSpace(got.ConversationId), strings.TrimSpace(conversationID)) {
				return errors.New("message does not belong to the specified conversation")
			}
		}
	}

	in := &msgdel.Input{Ids: []string{strings.TrimSpace(messageID)}}
	out := &msgdel.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodDelete, msgdel.PathURI)),
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

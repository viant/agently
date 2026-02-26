package conversation

import (
	"context"
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
	generatedfileread "github.com/viant/agently/pkg/agently/generatedfile/read"
	generatedfilewrite "github.com/viant/agently/pkg/agently/generatedfile/write"
	msgdel "github.com/viant/agently/pkg/agently/message/delete"
	messageread "github.com/viant/agently/pkg/agently/message/read"
	msgwrite "github.com/viant/agently/pkg/agently/message/write"
	modelcallwrite "github.com/viant/agently/pkg/agently/modelcall/write"
	payloadread "github.com/viant/agently/pkg/agently/payload/read"
	payloadwrite "github.com/viant/agently/pkg/agently/payload/write"
	toolread "github.com/viant/agently/pkg/agently/toolcall/read"
	toolcallwrite "github.com/viant/agently/pkg/agently/toolcall/write"
	turnread "github.com/viant/agently/pkg/agently/turn/read"
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
		if err := generatedfileread.DefineComponent(ctx, dao); err != nil {
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
		if _, err := toolread.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := turnwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := turnread.DefineNextQueuedComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := turnread.DefineActiveTurnComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := turnread.DefineTurnByIDComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := turnread.DefineQueuedCountComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if err := turnread.DefineQueuedListComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := payloadwrite.DefineComponent(ctx, dao); err != nil {
			initErr = err
			return
		}
		if _, err := generatedfilewrite.DefineComponent(ctx, dao); err != nil {
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
	if conversations != nil {
		debugf("PatchConversations start id=%q status=%q visibility=%q", strings.TrimSpace(conversations.Id), strings.TrimSpace(valueOrEmptyStr(conversations.Status)), strings.TrimSpace(valueOrEmptyStr(conversations.Visibility)))
	} else {
		debugf("PatchConversations start id=\"\" status=\"\" visibility=\"\" (nil input)")
	}
	conv := []*convw.Conversation{(*convw.Conversation)(conversations)}
	input := &convw.Input{Conversations: conv}
	out := &convw.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, convw.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		errorf("PatchConversations error id=%q err=%v", strings.TrimSpace(conversations.Id), err)
		return err
	}
	if len(out.Violations) > 0 {
		warnf("PatchConversations violation id=%q msg=%q", strings.TrimSpace(conversations.Id), out.Violations[0].Message)
		return errors.New(out.Violations[0].Message)
	}
	debugf("PatchConversations ok id=%q", strings.TrimSpace(conversations.Id))
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
	//TODO streamline and unify conversation casting
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
	debugf("PatchPayload start id=%q kind=%q mime=%q size_bytes=%d", strings.TrimSpace(payload.Id), strings.TrimSpace(payload.Kind), strings.TrimSpace(payload.MimeType), payload.SizeBytes)
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
		errorf("PatchPayload error id=%q err=%v", strings.TrimSpace(payload.Id), err)
		return err
	}
	if len(out.Violations) > 0 {
		warnf("PatchPayload violation id=%q msg=%q", strings.TrimSpace(payload.Id), out.Violations[0].Message)
		return errors.New(out.Violations[0].Message)
	}
	debugf("PatchPayload ok id=%q", strings.TrimSpace(payload.Id))
	return nil
}

func (s *Service) GetGeneratedFiles(ctx context.Context, input *generatedfileread.Input) ([]*generatedfileread.GeneratedFileView, error) {
	if s == nil || s.dao == nil {
		return nil, errors.New("conversation service not configured: dao is nil")
	}
	in := generatedfileread.Input{}
	if input != nil {
		in = *input
	}
	if in.Has == nil {
		in.Has = &generatedfileread.Has{}
	}
	out := &generatedfileread.Output{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(generatedfileread.URI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (s *Service) PatchGeneratedFile(ctx context.Context, generatedFile *generatedfilewrite.GeneratedFile) error {
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if generatedFile == nil {
		return errors.New("invalid generated file: nil")
	}
	debugf("PatchGeneratedFile start id=%q provider=%q mode=%q status=%q", strings.TrimSpace(generatedFile.ID), strings.TrimSpace(generatedFile.Provider), strings.TrimSpace(generatedFile.Mode), strings.TrimSpace(generatedFile.Status))
	input := &generatedfilewrite.Input{GeneratedFiles: []*generatedfilewrite.GeneratedFile{generatedFile}}
	out := &generatedfilewrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, generatedfilewrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		errorf("PatchGeneratedFile error id=%q err=%v", strings.TrimSpace(generatedFile.ID), err)
		return err
	}
	if len(out.Violations) > 0 {
		warnf("PatchGeneratedFile violation id=%q msg=%q", strings.TrimSpace(generatedFile.ID), out.Violations[0].Message)
		return errors.New(out.Violations[0].Message)
	}
	debugf("PatchGeneratedFile ok id=%q", strings.TrimSpace(generatedFile.ID))
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
	if message != nil {
		debugf("PatchMessage start id=%q convo=%q turn=%v role=%q type=%q status=%q", message.Id, message.ConversationID, valueOrEmpty(message.TurnID), strings.TrimSpace(message.Role), strings.TrimSpace(message.Type), strings.TrimSpace(message.Status))
	} else {
		debugf("PatchMessage start id=\"\" convo=\"\" turn=\"\" role=\"\" type=\"\" status=\"\" (nil input)")
	}
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
		errorf("PatchMessage error id=%q convo=%q err=%v", message.Id, message.ConversationID, err)
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
		warnf("PatchMessage violation id=%q convo=%q msg=%q", message.Id, message.ConversationID, out.Violations[0].Message)
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
	debugf("PatchMessage ok id=%q convo=%q", message.Id, message.ConversationID)
	return nil
}

// valueOrEmpty renders pointer values without exposing nil dereference in logs.
func valueOrEmpty[T any](p *T) interface{} {
	if p == nil {
		return ""
	}
	return *p
}

func valueOrEmptyStr(p *string) string {
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
	debugf("PatchModelCall start message_id=%q turn_id=%q provider=%q model=%q status=%q", strings.TrimSpace(modelCall.MessageID), strings.TrimSpace(valueOrEmptyStr(modelCall.TurnID)), strings.TrimSpace(modelCall.Provider), strings.TrimSpace(modelCall.Model), strings.TrimSpace(modelCall.Status))
	mc := (*modelcallwrite.ModelCall)(modelCall)
	input := &modelcallwrite.Input{ModelCalls: []*modelcallwrite.ModelCall{mc}}
	out := &modelcallwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, modelcallwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)

	if err != nil {
		errorf("PatchModelCall error message_id=%q err=%v", strings.TrimSpace(modelCall.MessageID), err)
		return err
	}
	if len(out.Violations) > 0 {
		warnf("PatchModelCall violation message_id=%q msg=%q", strings.TrimSpace(modelCall.MessageID), out.Violations[0].Message)
		return errors.New(out.Violations[0].Message)
	}
	debugf("PatchModelCall ok message_id=%q status=%q", strings.TrimSpace(modelCall.MessageID), strings.TrimSpace(modelCall.Status))
	return nil
}

func (s *Service) PatchToolCall(ctx context.Context, toolCall *convcli.MutableToolCall) error {
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if toolCall == nil {
		return errors.New("invalid toolCall: nil")
	}
	debugf("PatchToolCall start message_id=%q op_id=%q tool=%q status=%q", strings.TrimSpace(toolCall.MessageID), strings.TrimSpace(toolCall.OpID), strings.TrimSpace(toolCall.ToolName), strings.TrimSpace(toolCall.Status))
	tc := (*toolcallwrite.ToolCall)(toolCall)
	input := &toolcallwrite.Input{ToolCalls: []*toolcallwrite.ToolCall{tc}}
	out := &toolcallwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, toolcallwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		errorf("PatchToolCall error message_id=%q err=%v", strings.TrimSpace(toolCall.MessageID), err)
		return err
	}
	if len(out.Violations) > 0 {
		warnf("PatchToolCall violation message_id=%q msg=%q", strings.TrimSpace(toolCall.MessageID), out.Violations[0].Message)
		return errors.New(out.Violations[0].Message)
	}
	debugf("PatchToolCall ok message_id=%q status=%q", strings.TrimSpace(toolCall.MessageID), strings.TrimSpace(toolCall.Status))
	return nil
}

// ToolCallTraceByOp returns the persisted trace_id (LLM response.id anchor) for a tool call op_id
// scoped to a conversation. It returns an empty string when not found.
func (s *Service) ToolCallTraceByOp(ctx context.Context, conversationID, opID string) (string, error) {
	if s == nil || s.dao == nil {
		return "", errors.New("conversation service not configured: dao is nil")
	}
	in := &toolread.ByOpInput{ConversationId: strings.TrimSpace(conversationID), OpId: strings.TrimSpace(opID), Has: &toolread.ByOpInputHas{ConversationId: true, OpId: true}}
	out := &toolread.ByOpOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(toolread.PathURI), datly.WithInput(in)); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", nil
	}
	if out.Data[0] == nil || out.Data[0].TraceId == nil {
		return "", nil
	}
	return strings.TrimSpace(*out.Data[0].TraceId), nil
}

func (s *Service) PatchTurn(ctx context.Context, turn *convcli.MutableTurn) error {
	if s == nil || s.dao == nil {
		return errors.New("conversation service not configured: dao is nil")
	}
	if turn == nil {
		return errors.New("invalid turn: nil")
	}
	debugf("PatchTurn start id=%q convo=%q status=%q queue_seq=%v", strings.TrimSpace(turn.Id), strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.Status), valueOrEmpty(turn.QueueSeq))
	tr := (*turnwrite.Turn)(turn)
	input := &turnwrite.Input{Turns: []*turnwrite.Turn{tr}}
	out := &turnwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, turnwrite.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		errorf("PatchTurn error id=%q err=%v", strings.TrimSpace(turn.Id), err)
		return err
	}
	if len(out.Violations) > 0 {
		warnf("PatchTurn violation id=%q msg=%q", strings.TrimSpace(turn.Id), out.Violations[0].Message)
		return errors.New(out.Violations[0].Message)
	}
	debugf("PatchTurn ok id=%q status=%q", strings.TrimSpace(turn.Id), strings.TrimSpace(turn.Status))
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

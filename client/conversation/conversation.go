package conversation

import (
	"context"
	"unsafe"

	agconv "github.com/viant/agently/pkg/agently/conversation"

	"github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/agently/pkg/agently/conversation/write"
)

type Input conversation.ConversationInput

type MutableConversation write.Conversation

type Conversation conversation.ConversationView

func (c *Conversation) GetTranscript() Transcript {
	if c.Transcript == nil {
		return nil
	}
	return *(*Transcript)(unsafe.Pointer(&c.Transcript))
}

// GetRequest defines parameters to retrieve a conversation view.
type GetRequest struct {
	Id               string
	Since            string
	IncludeModelCall bool
	IncludeToolCall  bool
}

// GetResponse wraps the conversation view.
type GetResponse struct {
	Conversation *Conversation
}

// Service is a thin wrapper around API to support request/response types.
type Service struct{ api API }

func NewService(api API) *Service { return &Service{api: api} }

// Get fetches a conversation based on the request fields.
func (s *Service) Get(ctx context.Context, req GetRequest) (*GetResponse, error) {
	if s == nil || s.api == nil {
		return &GetResponse{Conversation: nil}, nil
	}
	var opts []Option
	if req.Since != "" {
		opts = append(opts, WithSince(req.Since))
	}
	if req.IncludeModelCall {
		opts = append(opts, WithIncludeModelCall(true))
	}
	if req.IncludeToolCall {
		opts = append(opts, WithIncludeToolCall(true))
	}
	conv, err := s.api.GetConversation(ctx, req.Id, opts...)
	if err != nil {
		return nil, err
	}
	return &GetResponse{Conversation: conv}, nil
}

type Option func(input *Input)

// WithSince sets the optional since parameter controlling transcript filtering.
func WithSince(since string) Option {
	return func(input *Input) {
		input.Since = since
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.Since = true
	}
}

func WithIncludeToolCall(include bool) Option {
	return func(input *Input) {
		input.IncludeToolCall = include
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.IncludeToolCall = true
	}
}

func WithIncludeModelCall(include bool) Option {
	return func(input *Input) {
		input.IncludeModelCal = include
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.IncludeModelCal = true
	}
}

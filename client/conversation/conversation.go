package conversation

import (
	"context"
	"unsafe"

	agconv "github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/agently/pkg/agently/tool"
)

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
type Service struct{ api Client }

func NewService(api Client) *Service { return &Service{api: api} }

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

// WithToolFeedSpec populates the transient FeedSpec list on the input
// so that OnRelation hooks can compute tool executions based on metadata.
func WithToolFeedSpec(ext []*tool.FeedSpec) Option {
	return func(input *Input) {
		input.FeedSpec = ext
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.FeedSpec = true
	}
}

// WithScheduledEq applies coalesce(t.scheduled,0) = value (0 or 1).
func WithScheduledEq(value int) Option {
	return func(input *Input) {
		input.Scheduled = value
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.Scheduled = true
	}
}

// WithScheduledOnly is a convenience for WithScheduledEq(1).
func WithScheduledOnly() Option { return WithScheduledEq(1) }

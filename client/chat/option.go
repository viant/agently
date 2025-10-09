package chat

import (
	agconv "github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/agently/pkg/agently/tool"
)

// Option mutates a generated `agconv.ConversationInput` to control reads.
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

// WithIncludeToolCall toggles inclusion of tool calls.
func WithIncludeToolCall(include bool) Option {
	return func(input *Input) {
		input.IncludeToolCall = include
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.IncludeToolCall = true
	}
}

// WithIncludeModelCall toggles inclusion of model calls.
func WithIncludeModelCall(include bool) Option {
	return func(input *Input) {
		input.IncludeModelCal = include
		if input.Has == nil {
			input.Has = &agconv.ConversationInputHas{}
		}
		input.Has.IncludeModelCal = true
	}
}

// WithToolFeedSpec populates transient extensions for tool-call computation.
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

package conversation

import agconv "github.com/viant/agently/pkg/agently/conversation"

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

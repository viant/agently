package conversation

type ConversationInputOption func(*ConversationInput)

func WithID(id string) ConversationInputOption {
	return func(input *ConversationInput) {
		input.Id = id
	}
}

// WithSince sets the optional since parameter on ConversationInput.
// Semantics are defined in the generated DQL; currently it constrains
// the related turn/message view according to the predicate in conversation.dql.
func WithSince(since string) ConversationInputOption {
	return func(input *ConversationInput) {
		input.Since = since
		if input.Has == nil {
			input.Has = &ConversationInputHas{}
		}
		input.Has.Since = true
	}
}

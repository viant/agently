package read

// ConversationInputOption mutates ConversationInput for query composition.
type ConversationInputOption func(*ConversationInput)

// WithID filters conversations by id.
func WithID(id string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.Id = id
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.Id = true
	}
}

// WithSummaryContains filters conversations by summary contains.
func WithSummaryContains(summary string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.Summary = summary
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.Summary = true
	}
}

// WithTitleContains filters conversations by title contains.
func WithTitleContains(title string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.Title = title
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.Title = true
	}
}

// WithAgentNameContains filters by agent_name contains.
func WithAgentNameContains(name string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.AgentName = name
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.AgentName = true
	}
}

// WithAgentID filters by agent_id equality.
func WithAgentID(agentID string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.AgentID = agentID
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.AgentID = true
	}
}

// WithVisibility filters by visibility equality.
func WithVisibility(vis string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.Visibility = vis
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.Visibility = true
	}
}

// WithCreatedByUserID filters by created_by_user_id equality.
func WithCreatedByUserID(userID string) ConversationInputOption {
	return func(in *ConversationInput) {
		in.CreatedByUserID = userID
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.CreatedByUserID = true
	}
}

// WithArchived filters by archived flag(s) using IN predicate.
func WithArchived(values ...int) ConversationInputOption {
	return func(in *ConversationInput) {
		in.Archived = values
		if in.Has == nil {
			in.Has = &ConversationInputHas{}
		}
		in.Has.Archived = true
	}
}

// WithInput replaces the entire input (advanced option).
func WithInput(src ConversationInput) ConversationInputOption {
	return func(in *ConversationInput) { *in = src }
}

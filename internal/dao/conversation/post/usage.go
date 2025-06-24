package post

// ConversationModelUsage represents aggregated token usage for a conversation
// broken down by the specific model that generated the tokens. The primary key
// is the composite (ConversationId, ModelName).
type ConversationModelUsage struct {
	ConversationId string `sqlx:"conversation_id,primaryKey" validate:"required"`
	ModelName      string `sqlx:"model_name,primaryKey" validate:"required"`

	InputTokens     int `sqlx:"input_tokens"`
	OutputTokens    int `sqlx:"output_tokens"`
	EmbeddingTokens int `sqlx:"embedding_tokens"`

	Has *ConversationModelUsageHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ConversationModelUsageHas struct {
	ConversationId  bool
	ModelName       bool
	InputTokens     bool
	OutputTokens    bool
	EmbeddingTokens bool
}

func (u *ConversationModelUsage) SetConversationId(v string) {
	u.ConversationId = v
	u.Has.ConversationId = true
}

func (u *ConversationModelUsage) SetModelName(v string) {
	u.ModelName = v
	u.Has.ModelName = true
}

func (u *ConversationModelUsage) SetInputTokens(v int) {
	u.InputTokens = v
	u.Has.InputTokens = true
}

func (u *ConversationModelUsage) SetOutputTokens(v int) {
	u.OutputTokens = v
	u.Has.OutputTokens = true
}

func (u *ConversationModelUsage) SetEmbeddingTokens(v int) {
	u.EmbeddingTokens = v
	u.Has.EmbeddingTokens = true
}

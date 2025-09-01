package write

type Usage struct {
	Id                   string    `sqlx:"id,primaryKey" validate:"required"`
	UsageInputTokens     int       `sqlx:"usage_input_tokens"`
	UsageOutputTokens    int       `sqlx:"usage_output_tokens"`
	UsageEmbeddingTokens int       `sqlx:"usage_embedding_tokens"`
	Has                  *UsageHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type UsageHas struct {
	Id                   bool
	UsageInputTokens     bool
	UsageOutputTokens    bool
	UsageEmbeddingTokens bool
}

func (u *Usage) SetConversationID(v string) { u.Id = v; ensureHas(&u.Has); u.Has.Id = true }
func (u *Usage) SetUsageInputTokens(v int) {
	u.UsageInputTokens = v
	ensureHas(&u.Has)
	u.Has.UsageInputTokens = true
}
func (u *Usage) SetUsageOutputTokens(v int) {
	u.UsageOutputTokens = v
	ensureHas(&u.Has)
	u.Has.UsageOutputTokens = true
}
func (u *Usage) SetUsageEmbeddingTokens(v int) {
	u.UsageEmbeddingTokens = v
	ensureHas(&u.Has)
	u.Has.UsageEmbeddingTokens = true
}

func ensureHas(h **UsageHas) {
	if *h == nil {
		*h = &UsageHas{}
	}
}

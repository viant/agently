package config

type Defaults struct {
	Model    string
	Embedder string
	Agent    string

	// ---- Conversation summary defaults (optional) -------------------
	// When empty the runtime falls back to hard-coded defaults.
	SummaryModel  string `yaml:"summaryModel" json:"summaryModel"`
	SummaryPrompt string `yaml:"summaryPrompt" json:"summaryPrompt"`
	SummaryLastN  int    `yaml:"summaryLastN" json:"summaryLastN"`

	// ---- Tool-call result controls (grouped) ---------------------
	ToolCallResult ToolCallResultDefaults `yaml:"toolCallResult" json:"toolCallResult"`

	// ---- Match defaults (optional) -------------------------------
	Match MatchDefaults `yaml:"match" json:"match"`
}

// ToolCallResultDefaults groups tool-call result presentation and processing settings.
type ToolCallResultDefaults struct {
	PreviewLimit   int    `yaml:"previewLimit" json:"previewLimit"`
	SummarizeChunk int    `yaml:"summarizeChunk" json:"summarizeChunk"`
	MatchChunk     int    `yaml:"matchChunk" json:"matchChunk"`
	SummaryModel   string `yaml:"summaryModel" json:"summaryModel"`
	EmbeddingModel string `yaml:"embeddingModel" json:"embeddingModel"`
	// Optional system guide document (path or URL) injected when overflow occurs.
	SystemGuidePath string `yaml:"systemGuidePath" json:"systemGuidePath"`
}

// MatchDefaults groups retrieval/matching defaults
type MatchDefaults struct {
	// MaxFiles is the default per-location cap used by auto/full decision
	// when a knowledge/MCP entry does not specify MaxFiles. When zero,
	// the runtime falls back to hard-coded default (5).
	MaxFiles int `yaml:"maxFiles" json:"maxFiles"`
}

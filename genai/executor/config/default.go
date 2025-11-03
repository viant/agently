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
	PreviewSettings PreviewSettings `yaml:"previewSettings" json:"previewSettings"`

	// ---- Execution timeouts -------------------------------------
	// ToolCallTimeoutSec sets the default per-tool execution timeout in seconds.
	// When zero or missing, runtime falls back to a built-in default.
	ToolCallTimeoutSec int `yaml:"toolCallTimeoutSec,omitempty" json:"toolCallTimeoutSec,omitempty"`
	// ElicitationTimeoutSec caps how long the agent waits for an elicitation
	// (assistant- or tool-originated) before auto-declining. When zero, no
	// special timeout is applied (waits until the turn/request is canceled).
	ElicitationTimeoutSec int `yaml:"elicitationTimeoutSec,omitempty" json:"elicitationTimeoutSec,omitempty"`

	// ---- Match defaults (optional) -------------------------------
	Match MatchDefaults `yaml:"match" json:"match"`
}

// PreviewSettings groups tool-call result presentation and processing settings.
type PreviewSettings struct {
	Limit int `yaml:"limit" json:"limit"`

	AgedLimit int `yaml:"agedLimit" json:"agedLimit"`

	// How far back until we switch the UI to an aged preview.
	AgedAfterSteps int `yaml:"agedAfterSteps" json:"agedAfterSteps"`

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

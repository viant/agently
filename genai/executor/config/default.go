package config

type Defaults struct {
	Model    string
	Embedder string
	Agent    string

	// ---- Agent routing defaults (optional) -------------------------
	// When Agent == "auto", the runtime may use these settings to pick a concrete
	// agent for the turn using an LLM-based classifier.
	AgentRouter AgentRouterDefaults `yaml:"agentRouter" json:"agentRouter"`

	// ---- Conversation summary defaults (optional) -------------------
	// When empty the runtime falls back to hard-coded defaults.
	SummaryModel  string `yaml:"summaryModel" json:"summaryModel"`
	SummaryPrompt string `yaml:"summaryPrompt" json:"summaryPrompt"`
	SummaryLastN  int    `yaml:"summaryLastN" json:"summaryLastN"`

	// ---- Tool-call result controls (grouped) ---------------------
	PreviewSettings PreviewSettings `yaml:"previewSettings" json:"previewSettings"`

	ToolCallMaxResults int `yaml:"toolCallMaxResults" json:"toolCallMaxResults"`

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

	// ---- Resources defaults (optional) ---------------------------
	Resources ResourcesDefaults `yaml:"resources,omitempty" json:"resources,omitempty"`
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
	// SummaryThresholdBytes controls when internal/message:summarize is
	// exposed for overflowed messages. When zero or negative, any
	// overflowed message may use summarize.
	SummaryThresholdBytes int `yaml:"summaryThresholdBytes,omitempty" json:"summaryThresholdBytes,omitempty"`
}

// MatchDefaults groups retrieval/matching defaults
type MatchDefaults struct {
	// MaxFiles is the default per-location cap used by auto/full decision
	// when a knowledge/MCP entry does not specify MaxFiles. When zero,
	// the runtime falls back to hard-coded default (5).
	MaxFiles int `yaml:"maxFiles" json:"maxFiles"`
}

// ResourcesDefaults defines default resource roots and presentation hints.
type ResourcesDefaults struct {
	// Locations are root URIs or paths (relative to workspace) such as
	// "documents/", "file:///abs/path", or "mcp:server:/prefix".
	Locations []string `yaml:"locations,omitempty" json:"locations,omitempty"`
	// TrimPath optionally trims this prefix from presented URIs.
	TrimPath string `yaml:"trimPath,omitempty" json:"trimPath,omitempty"`
	// SummaryFiles lookup order for root descriptions.
	SummaryFiles []string `yaml:"summaryFiles,omitempty" json:"summaryFiles,omitempty"`
}

// AgentRouterDefaults controls the LLM-based agent classifier used for auto routing.
type AgentRouterDefaults struct {
	// Model is the model used for routing decisions. When empty, runtime falls back
	// to the conversation default model or Defaults.Model.
	Model string `yaml:"model,omitempty" json:"model,omitempty"`
	// Prompt optionally overrides the default system prompt used by the router.
	Prompt string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	// OutputKey controls the JSON field name the classifier should output.
	// Examples: "agentId" (default), "agent_id".
	OutputKey string `yaml:"outputKey,omitempty" json:"outputKey,omitempty"`
}

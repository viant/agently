package provider

import basecfg "github.com/viant/agently/genai/llm/provider/base"

type Options struct {
	Model          string                 `yaml:"model,omitempty" json:"model,omitempty"`
	Provider       string                 `yaml:"provider,omitempty" json:"provider,omitempty"`
	APIKeyURL      string                 `yaml:"apiKeyURL,omitempty" json:"APIKeyURL,omitempty"`
	EnvKey         string                 `yaml:"envKey,omitempty" json:"envKey,omitempty"` // environment variable key to use for API key
	CredentialsURL string                 `yaml:"credentialsURL,omitempty" json:"credentialsURL,omitempty"`
	URL            string                 `yaml:"Paths,omitempty" json:"Paths,omitempty"`
	ProjectID      string                 `yaml:"projectID,omitempty" json:"projectID,omitempty"`
	Temperature    *float64               `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens      int                    `yaml:"maxTokens,omitempty" json:"maxTokens,omitempty"`
	TopP           float64                `yaml:"topP,omitempty" json:"topP,omitempty"`
	Meta           map[string]interface{} `yaml:"meta,omitempty" json:"meta,omitempty"`
	Region         string                 `yaml:"region,omitempty" json:"region,omitempty"`
	UsageListener  basecfg.UsageListener  `yaml:"-" json:"-"`

	// ---- Pricing ----
	// Cost per 1,000 input tokens in USD (or chosen currency). Optional.
	InputTokenPrice float64 `yaml:"inputTokenPrice,omitempty" json:"inputTokenPrice,omitempty"`
	// Cost per 1,000 output/completion tokens.
	OutputTokenPrice float64 `yaml:"outputTokenPrice,omitempty" json:"outputTokenPrice,omitempty"`
	// Cost per 1,000 tokens served from cache (no LLM call).
	CachedTokenPrice float64 `yaml:"cachedTokenPrice,omitempty" json:"cachedTokenPrice,omitempty"`

	// Preview limit for tool results when this model is used (bytes).
	ToolResultPreviewLimit int `yaml:"toolResultPreviewLimit,omitempty" json:"toolResultPreviewLimit,omitempty"`
}

package provider

import basecfg "github.com/viant/agently/genai/llm/provider/base"

// ChatGPTOAuthOptions configures ChatGPT OAuth (Code+PKCE) based credential acquisition.
// This is intended for flows that mint an OpenAI API key (or equivalent) via token exchange.
type ChatGPTOAuthOptions struct {
	// OAuth client configuration (client id / secret), stored as a scy resource.
	// Typically points to a secret containing `cred.Oauth2Config`.
	ClientURL string `yaml:"clientURL,omitempty" json:"clientURL,omitempty"`

	// Token state persistence location, stored as a scy resource.
	// Intended to store refresh/access/id tokens and related metadata.
	TokensURL string `yaml:"tokensURL,omitempty" json:"tokensURL,omitempty"`

	// OAuth issuer, e.g. "https://auth.openai.com".
	Issuer string `yaml:"issuer,omitempty" json:"issuer,omitempty"`

	// Optional restriction for which ChatGPT workspace/account may be used.
	AllowedWorkspaceID string `yaml:"allowedWorkspaceID,omitempty" json:"allowedWorkspaceID,omitempty"`
}

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
	UserAgent      string                 `yaml:"userAgent,omitempty" json:"userAgent,omitempty"`
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

	// ---- Safety Limits ----
	// SafeEffectiveInputTokens defines a conservative safe input token count
	// (excludes model output and provider overhead). Intended for request planning.
	SafeEffectiveInputTokens int `yaml:"safeEffectiveInputTokens,omitempty" json:"safeEffectiveInputTokens,omitempty"`

	// ContextContinuation explicitly enables/disables provider continuation
	// for models (i.e. via previous_response_id for openai).
	ContextContinuation *bool `json:"contextContinuation,omitempty" yaml:"contextContinuation,omitempty"`

	EnableContinuationFormat bool `json:"enableContinuationFormat,omitempty" yaml:"enableContinuationFormat,omitempty"`

	// ChatGPTOAuth configures ChatGPT OAuth based credential acquisition.
	ChatGPTOAuth *ChatGPTOAuthOptions `yaml:"chatgptOAuth,omitempty" json:"chatgptOAuth,omitempty"`
}

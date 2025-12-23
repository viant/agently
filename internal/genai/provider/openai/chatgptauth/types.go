package chatgptauth

import "time"

// TokenState persists ChatGPT OAuth tokens and an optionally minted OpenAI API key.
// It is stored via scy at the configured tokensURL.
type TokenState struct {
	IDToken      string `json:"id_token,omitempty" yaml:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty" yaml:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty" yaml:"refresh_token,omitempty"`

	LastRefresh time.Time `json:"last_refresh,omitempty" yaml:"last_refresh,omitempty"`

	OpenAIAPIKey      string    `json:"openai_api_key,omitempty" yaml:"openai_api_key,omitempty"`
	OpenAIAPIKeyAt    time.Time `json:"openai_api_key_at,omitempty" yaml:"openai_api_key_at,omitempty"`
	OpenAIAPIKeyTTLMS int64     `json:"openai_api_key_ttl_ms,omitempty" yaml:"openai_api_key_ttl_ms,omitempty"`
}

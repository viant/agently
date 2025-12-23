package provider

import "net/http"

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
	Model          string   `yaml:"model,omitempty" json:"model,omitempty"`
	Provider       string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	APIKeyURL      string   `yaml:"apiKeyURL,omitempty" json:"apiKeyURL,omitempty"`
	CredentialsURL string   `yaml:"credentialsURL,omitempty" json:"credentialsURL,omitempty"`
	URL            string   `yaml:"url,omitempty" json:"url,omitempty"`
	ProjectID      string   `yaml:"projectID,omitempty" json:"projectID,omitempty"`
	Location       string   `yaml:"location,omitempty" json:"location,omitempty"`
	Scopes         []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`

	// ChatGPTOAuth configures ChatGPT OAuth based credential acquisition.
	ChatGPTOAuth *ChatGPTOAuthOptions `yaml:"chatgptOAuth,omitempty" json:"chatgptOAuth,omitempty"`

	httpClient    *http.Client                    `yaml:"-" json:"-"`
	usageListener func(data []string, tokens int) `yaml:"-" json:"-"` // usageListener is a callback function to handle token usage
}

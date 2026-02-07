package openai

import (
	"net/http"
	"time"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// ClientOption mutates an OpenAI Client instance.
type ClientOption func(*Client)

func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { basecfg.WithBaseURL(baseURL)(&c.Config) }
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { basecfg.WithHTTPClient(httpClient)(&c.Config) }
}

func WithModel(model string) ClientOption {
	return func(c *Client) { basecfg.WithModel(model)(&c.Config) }
}

func WithTimeout(timeoutSeconds int) ClientOption {
	return func(c *Client) { basecfg.WithTimeout(time.Duration(timeoutSeconds) * time.Second)(&c.Config) }
}

// WithMaxTokens sets a default max_tokens that will be applied to any
// Generate request that does not explicitly specify MaxTokens in the options.
func WithMaxTokens(max int) ClientOption {
	return func(c *Client) { c.MaxTokens = max }
}

// WithTemperature sets a default temperature applied when a Generate request
// does not specify it.
func WithTemperature(temp float64) ClientOption {
	return func(c *Client) { c.Temperature = &temp }
}

// WithUsageListener assigns token usage listener to the client.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.Config.UsageListener = l }
}

// WithAPIKeyProvider configures a resolver used to obtain an API key at call time.
// This is intended for auth flows that mint or refresh API keys dynamically.
func WithAPIKeyProvider(provider APIKeyProvider) ClientOption {
	return func(c *Client) { c.APIKeyProvider = provider }
}

// WithContextContinuation sets a client-level toggle for server-side context
// continuation (continuation by response_id) when supported by the provider.
func WithContextContinuation(enabled *bool) ClientOption {
	return func(c *Client) { c.ContextContinuation = enabled }
}

// WithUserAgent sets a User-Agent override for OpenAI requests.
// The override is applied only when the value starts with "openai" (case-insensitive).
func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) { c.UserAgent = userAgent }
}

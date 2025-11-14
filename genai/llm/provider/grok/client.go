package grok

import (
	"net/http"
	"time"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// Default endpoint for xAI Grok API
const defaultBaseURL = "https://api.x.ai/v1"

// Client represents a Grok (xAI) API client
type Client struct {
	basecfg.Config
	APIKey string
}

// ClientOption mutates the underlying base Config.
type ClientOption func(*Client)

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { basecfg.WithHTTPClient(httpClient)(&c.Config) }
}

// WithBaseURL overrides the default API base URL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { basecfg.WithBaseURL(baseURL)(&c.Config) }
}

// WithModel selects the model name.
func WithModel(model string) ClientOption {
	return func(c *Client) { basecfg.WithModel(model)(&c.Config) }
}

// WithUsageListener registers a usage listener.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = l }
}

// NewClient creates a new Grok client with the given API key and model
func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: basecfg.Config{
			HTTPClient: &http.Client{Timeout: 60 * time.Minute},
			BaseURL:    defaultBaseURL,
			Model:      model,
		},
		APIKey: apiKey,
	}

	for _, opt := range options {
		opt(client)
	}
	return client
}

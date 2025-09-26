package openai

import (
	"github.com/viant/agently/genai/embedder/provider/base"
	"net/http"
)

// ClientOption mutates an OpenAI Client instance.
type ClientOption func(*Client)

// Generic helpers delegate to the shared implementation that operates on the
// embedded *base.Config*.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { base.WithBaseURL(baseURL)(&c.Config) }
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { base.WithHTTPClient(httpClient)(&c.Config) }
}

func WithModel(model string) ClientOption {
	return func(c *Client) { base.WithModel(model)(&c.Config) }
}

// Provider-specific option.
func WithUsageListener(listener base.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = listener }
}

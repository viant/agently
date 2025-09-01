package ollama

import (
	"github.com/viant/agently/genai/embedder/provider/base"
	"net/http"
)

// ClientOption mutates the provider client instance.
type ClientOption func(*Client)

// Generic helpers delegate to the shared implementation operating on
// *base.Config*.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { base.WithBaseURL(baseURL)(&c.Config) }
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { base.WithHTTPClient(httpClient)(&c.Config) }
}

func WithModel(model string) ClientOption {
	return func(c *Client) { base.WithModel(model)(&c.Config) }
}

func WithUsageListener(listener base.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = listener }
}

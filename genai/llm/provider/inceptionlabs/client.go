package inceptionlabs

import (
	"net/http"
	"time"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// Client represents an InceptionLabs API client
type Client struct {
	basecfg.Config
	APIKey string
}

type ClientOption func(*Client)

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { basecfg.WithHTTPClient(httpClient)(&c.Config) }
}

func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { basecfg.WithBaseURL(baseURL)(&c.Config) }
}

func WithModel(model string) ClientOption {
	return func(c *Client) { basecfg.WithModel(model)(&c.Config) }
}

// WithUsageListener registers a callback to receive token usage information.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = l }
}

// NewClient creates a new InceptionLabs client with the given API key and model
func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: basecfg.Config{
			HTTPClient: &http.Client{Timeout: 30 * time.Second},
			BaseURL:    inceptionLabsEndpoint,
			Model:      model,
		},
		APIKey: apiKey,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}
	return client
}

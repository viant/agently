package gemini

import (
	"net/http"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

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

func WithVersion(version string) ClientOption {
	return func(c *Client) { c.Version = version }
}

// WithMaxTokens sets a default maxOutputTokens applied when request omits it.
func WithMaxTokens(max int) ClientOption {
	return func(c *Client) { c.MaxTokens = max }
}

// WithTemperature sets a default temperature applied when request omits it.
func WithTemperature(temp float64) ClientOption {
	return func(c *Client) { c.Temperature = &temp }
}

// WithUsageListener registers a callback to receive token usage information.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = l }
}

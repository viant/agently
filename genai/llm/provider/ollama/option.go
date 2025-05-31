package ollama

import (
	"net/http"
	"time"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

type ClientOption func(*Client)

func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { basecfg.WithBaseURL(baseURL)(&c.Config) }
}

func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) { basecfg.WithHTTPClient(client)(&c.Config) }
}

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) { basecfg.WithTimeout(timeout)(&c.Config) }
}

func WithModel(model string) ClientOption {
	return func(c *Client) { basecfg.WithModel(model)(&c.Config) }
}

// WithUsageListener registers a callback to receive token usage information.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = l }
}

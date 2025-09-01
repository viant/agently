package base

import (
	"net/http"
	"time"
)

// Config aggregates common client parameters used by all LLM providers.  It is
// embedded into every concrete provider.Client to remove the need for
// per-package boiler-plate.
type Config struct {
	BaseURL    string
	HTTPClient *http.Client
	Model      string
	Timeout    time.Duration

	// UsageListener, when set, receives token usage information for each
	// successful model invocation.  It can be used to aggregate cost metrics
	// per request.
	UsageListener UsageListener
}

// ClientOption mutates Config; providers expose it via type alias so that users
// can continue to call e.g. *openai.WithBaseURL(...)*.
type ClientOption func(*Config)

// WithBaseURL overrides the default endpoint of the provider.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Config) {
		if baseURL != "" {
			c.BaseURL = baseURL
		}
	}
}

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Config) {
		if client != nil {
			c.HTTPClient = client
		}
	}
}

// WithModel selects the model name.
func WithModel(model string) ClientOption {
	return func(c *Config) {
		if model != "" {
			c.Model = model
		}
	}
}

// WithTimeout sets request timeout (mainly used by local providers such as
// Ollama).
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Config) {
		if timeout > 0 {
			c.Timeout = timeout
		}
	}
}

// WithUsageListener registers a callback to receive token usage metrics.
func WithUsageListener(l UsageListener) ClientOption {
	return func(c *Config) {
		c.UsageListener = l
	}
}

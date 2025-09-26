package base

import "net/http"

// Config groups the fields that are common to every embedder provider Client.
//
// Several provider implementations (OpenAI, Ollama, VertexAI, …) were declaring
// the same set of fields (BaseURL, HTTPClient, Model) independently in their
// dedicated client structs and duplicating functional-option helpers to mutate
// them.  Config centralises those shared attributes so that every provider can
// embed *base.Config* and inherit the fields automatically.  Generic
// functional-options defined in this package operate on Config and therefore can
// be reused by all providers, eliminating the previous boiler-plate.
type Config struct {
	// BaseURL is the root URL of the remote embedding service (e.g. https://api.openai.com).
	BaseURL string

	// HTTPClient performs the underlying HTTP calls.  When nil the provider
	// should fall back to *http.DefaultClient* or create its own with sensible
	// timeout.
	HTTPClient *http.Client

	// Model identifies the embedding model (e.g. "text-embedding-3-small") to
	// be used by the provider.  The concrete provider may fall back to its own
	// default if Model is left empty.
	Model string
}

// ClientOption mutates Config.  Each provider may alias this type so that its
// constructor accepts a consistent set of generic options alongside any
// provider-specific ones.
type ClientOption func(*Config)

// WithBaseURL overrides the default endpoint URL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Config) {
		if baseURL != "" {
			c.BaseURL = baseURL
		}
	}
}

// WithHTTPClient injects a custom *http.Client* (for timeouts, transport, …).
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Config) {
		if httpClient != nil {
			c.HTTPClient = httpClient
		}
	}
}

// WithModel sets/overrides the model name.
func WithModel(model string) ClientOption {
	return func(c *Config) {
		if model != "" {
			c.Model = model
		}
	}
}

// WithUsageListener registers a callback that will be triggered every time the
// provider successfully creates an embedding.  The listener is stored on the
// generic Config so that the same functional-option can be reused by every
// concrete provider without duplicating boiler-plate.
// Note: UsageListener is not part of the generic Config because it lives on
// the embedded *base.Client*.  Each provider exposes its own
// WithUsageListener option to set that field directly – keeping the generic
// option set focused on connection/model parameters only.

// WithUsageListener assigns a UsageListener that will be copied to the concrete
// provider client (which embeds *base.Client) after construction.

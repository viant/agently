package openai

import (
	"net/http"
	"os"
	"time"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// Client represents an OpenAI API client
type Client struct {
	basecfg.Config
	APIKey string

	// Defaults applied when GenerateRequest.Options is nil or leaves the
	// respective field unset.
	MaxTokens   int
	Temperature *float64
}

// NewClient creates a new OpenAI client with the given API key and model
func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: basecfg.Config{
			HTTPClient: &http.Client{Timeout: 15 * time.Minute}, //TODO: make it configurable
			BaseURL:    openAIEndpoint,
			Model:      model,
		},
		APIKey: apiKey,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	if client.APIKey == "" {
		client.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	return client
}

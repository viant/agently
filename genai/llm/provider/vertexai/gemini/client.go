package gemini

import (
	"fmt"
	"net/http"
	"os"
	"time"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// Client represents a Gemini API client
type Client struct {
	basecfg.Config
	APIKey  string
	Version string

	MaxTokens   int
	Temperature *float64
}

// NewClient creates a new Gemini client with the given API key and model resource name
// model should be the full resource path, e.g., "projects/{project}/locations/{location}/models/{model}"
func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: basecfg.Config{
			HTTPClient: &http.Client{Timeout: 30 * time.Minute},
			Model:      model,
		},
		APIKey: apiKey,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	if client.APIKey == "" {
		client.APIKey = os.Getenv("GEMINI_API_KEY")
	}
	if client.Version == "" {
		client.Version = "v1beta"
		////https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent
	}
	if client.BaseURL == "" {
		client.BaseURL = fmt.Sprintf(geminiEndpoint, client.Version)
	}
	return client
}

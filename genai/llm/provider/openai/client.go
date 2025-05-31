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
}

// NewClient creates a new OpenAI client with the given API key and model
func NewClient(apiKey, model string, options ...ClientOption) *Client {
    client := &Client{
        Config: basecfg.Config{
            HTTPClient: &http.Client{Timeout: 30 * time.Second},
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

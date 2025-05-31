package ollama

import (
    "context"
    "net/http"
    "time"

    basecfg "github.com/viant/agently/genai/llm/provider/base"
)

const (
	defaultBaseURL = "http://localhost:11434"
	defaultTimeout = 120 * time.Second
)

// Client represents an Ollama API client
type Client struct {
    basecfg.Config
    Timeout time.Duration
}

// NewClient creates a new Ollama client
func NewClient(ctx context.Context, model string, options ...ClientOption) (*Client, error) {
    client := &Client{
        Config: basecfg.Config{
            BaseURL: defaultBaseURL,
            Model:   model,
            HTTPClient: &http.Client{
                Transport: &http.Transport{
                    TLSHandshakeTimeout:   10 * time.Second,
                    IdleConnTimeout:       10 * time.Second,
                    ResponseHeaderTimeout: defaultTimeout,
                },
                Timeout: 0,
            },
        },
        Timeout: defaultTimeout,
    }

	// Apply options
    for _, option := range options {
        option(client)
    }

	return client, nil
}

// PullModel pulls a model from Ollama
func (c *Client) PullModel(ctx context.Context, modelName string) (*PullResponse, error) {
	req := &PullRequest{
		Name: modelName,
	}
	return c.sendPullRequest(ctx, req)
}

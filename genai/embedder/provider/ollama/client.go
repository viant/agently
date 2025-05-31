package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/embedder/provider/base"
	"io"
	"net/http"
	"time"
)

// Client represents an Ollama API client for embeddings
type Client struct {
	base.Client
	base.Config
}

// NewClient creates a new Ollama embeddings client with the given model
func NewClient(model string, options ...ClientOption) *Client {
	client := &Client{
		Config: base.Config{
			HTTPClient: &http.Client{
				Timeout: 30 * time.Second,
			},
			BaseURL: ollamaEndpoint,
			Model:   model,
		},
	}
	client.Embedder = client

	for _, option := range options {
		option(client)
	}
	if client.Model == "" {
		client.Model = defaultEmbeddingModel
	}

	return client
}

// Embed creates embeddings for the given texts
func (c *Client) Embed(ctx context.Context, data []string) (vector [][]float32, totalTokens int, err error) {
	// Adapt the request
	reqs := AdaptRequest(data, c.Model)
	for _, req := range reqs {
		// Marshal the request to JSON
		reqBody, err := json.Marshal(req)
		if err != nil {
			return nil, 0, fmt.Errorf("error marshaling request: %w", err)
		}

		// Create the HTTP request
		httpReq, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			c.BaseURL+embeddingsEndpoint,
			bytes.NewReader(reqBody),
		)
		if err != nil {
			return nil, 0, fmt.Errorf("error creating request: %w", err)
		}

		// Set headers
		httpReq.Header.Set("Content-Type", "application/json")

		// Send the request
		resp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			return nil, 0, fmt.Errorf("error sending request: %w", err)
		}
		defer resp.Body.Close()

		// Check for error status
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, 0, fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
		}

		// Read the response body
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, 0, fmt.Errorf("error reading response: %w", err)
		}

		// Decode the response
		var embeddingResp Response
		if err := json.Unmarshal(data, &embeddingResp); err != nil {
			return nil, 0, fmt.Errorf("error decoding response: %w", err)
		}
		AdaptResponse(&embeddingResp, c.Model, &vector, &totalTokens)
	}
	return vector, totalTokens, nil
}

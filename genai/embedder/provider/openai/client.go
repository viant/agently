package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/embedder/provider/base"
	"io"
	"net/http"
	"os"
	"time"
)

// Client represents an OpenAI API client for embeddings
type Client struct {
	base.Client
	base.Config // embeds BaseURL, HTTPClient, Find
	APIKey      string
}

// NewClient creates a new OpenAI embeddings client with the given API key and model

func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: base.Config{
			HTTPClient: &http.Client{
				Timeout: 30 * time.Second,
			},
			BaseURL: openAIEndpoint,
			Model:   model,
		},
		APIKey: apiKey,
	}
	client.Embedder = client

	// Apply generic options – each option mutates the embedded Config
	for _, option := range options {
		option(client)
	}

	// Use environment variable if API key is not provided
	if client.APIKey == "" {
		client.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	// Use default model if not provided
	if client.Model == "" {
		client.Model = defaultEmbeddingModel
	}

	return client
}

// Embed creates embeddings for the given texts
func (c *Client) Embed(ctx context.Context, texts []string) (vector [][]float32, totalTokens int, err error) {
	// Adapt the request
	req := AdaptRequest(texts, c.Model)

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
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	// Send the request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error.Message != "" {
			return nil, 0, fmt.Errorf("API error (%s): %s", errResp.Error.Type, errResp.Error.Message)
		}
		return nil, 0, fmt.Errorf("API error: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("error reading response: %w", err)
	}

	// Decode the response
	var embeddingResp Response
	if err := json.Unmarshal(data, &embeddingResp); err != nil {
		return nil, 0, fmt.Errorf("error decoding response: %w", err)
	}

	// Adapt the response
	AdaptResponse(&embeddingResp, c.Model, &vector, &totalTokens)
	return vector, totalTokens, nil
}

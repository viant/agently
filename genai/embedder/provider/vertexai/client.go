package vertexai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/embedder/provider/base"
	"github.com/viant/scy/auth/gcp"
	gcpclient "github.com/viant/scy/auth/gcp/client"
	"io"
	"log"
	"net/http"
	"time"
)

// Client represents a VertexAI API client for embeddings
type Client struct {
	base.Client
	base.Config

	ProjectID     string
	ProjectNumber int
	Location      string
	MaxRetries    int
	authService   *gcp.Service
	scopes        []string
}

// NewClient creates a new VertexAI embeddings client with the given project ID and model
func NewClient(ctx context.Context, projectID, model string, options ...ClientOption) (*Client, error) {
	client := &Client{
		Config: base.Config{
			HTTPClient: &http.Client{
				Timeout: 30 * time.Second,
			},
			Model: model,
		},
		ProjectID:  projectID,
		Location:   defaultLocation,
		scopes:     []string{"https://www.googleapis.com/auth/cloud-platform"},
		MaxRetries: 2,
	}
	client.Embedder = client
	// Apply options
	for _, option := range options {
		option(client)
	}

	// Use default model if not provided
	if client.Model == "" {
		client.Model = defaultEmbeddingModel
	}

	// Initialize auth service if not provided
	if client.authService == nil {
		client.authService = gcp.New(gcpclient.NewGCloud())
	}

	// Create authenticated HTTP client
	var err error
	client.HTTPClient, err = client.authService.AuthClient(ctx, client.scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth client: %w", err)
	}

	if client.ProjectNumber == 0 {
		err = client.loadProjectNumber()
		if err != nil {
			return nil, err
		}
	}
	return client, nil
}

func (c *Client) loadProjectNumber() error {
	resp, err := c.HTTPClient.Get(fmt.Sprintf(projectMetaEndpoint, c.ProjectID))
	if err != nil {
		return fmt.Errorf("failed to get project ID: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read project ID response: %w", err)
	}
	var project Project // Using struct from above example
	err = json.Unmarshal(body, &project)
	if err != nil {
		log.Fatalf("Failed to unmarshal JSON response: %v", err)
	}
	c.ProjectNumber = project.Id()
	return err
}

// Embed creates embeddings for the given texts
func (c *Client) Embed(ctx context.Context, texts []string) (vector [][]float32, totalTokens int, err error) {
	// Validate required fields
	if c.ProjectNumber == 0 {
		return nil, 0, fmt.Errorf("project ID is required")
	}

	if c.Model == "" {
		return nil, 0, fmt.Errorf("model is required")
	}

	// Adapt the request
	req := AdaptRequest(texts, c.Model)

	// Marshal the request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error marshaling request: %w", err)
	}

	// Create the URL
	apiURL := fmt.Sprintf(vertexAIEndpoint, c.Location, c.ProjectNumber, c.Location, c.Model)

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		apiURL,
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")

	// Send the request with retries
	var resp *http.Response
	var lastErr error
	for i := 0; i <= c.MaxRetries; i++ {
		resp, err = c.HTTPClient.Do(httpReq)
		if err == nil {
			break
		}
		lastErr = err
		// Simple retry with exponential backoff
		if i < c.MaxRetries {
			time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("error sending request after %d retries: %w", c.MaxRetries, lastErr)
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
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

	// Adapt the response
	AdaptResponse(&embeddingResp, c.Model, &vector, &totalTokens)
	return vector, totalTokens, nil
}

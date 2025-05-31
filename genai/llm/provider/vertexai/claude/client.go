package claude

import (
    "context"
    "fmt"
    basecfg "github.com/viant/agently/genai/llm/provider/base"
    "github.com/viant/scy/auth/gcp"
    gcpclient "github.com/viant/scy/auth/gcp/client"
)

// Client represents a Claude API client for Vertex AI
type Client struct {
    basecfg.Config
    ProjectID        string
    Location         string
    AnthropicVersion string
    authService      *gcp.Service
    scopes           []string
    MaxRetries       int
}

// NewClient creates a new Claude client with the given project ID, location, and model
func NewClient(ctx context.Context, model string, options ...ClientOption) (*Client, error) {
    ret := &Client{
        Config: basecfg.Config{Model: model},
        Location:         defaultLocation,
        AnthropicVersion: defaultAnthropicVersion,
        MaxRetries:       2,
        scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
    }

	// Apply options
    for _, option := range options {
        option(ret)
    }

    if ret.authService == nil {
        ret.authService = gcp.New(gcpclient.NewGCloud())
    }
	var err error
    ret.HTTPClient, err = ret.authService.AuthClient(ctx, ret.scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth client: %w", err)
	}
	return ret, err
}

// GetEndpointURL returns the full endpoint URL for the API
func (c *Client) GetEndpointURL() string {
	return fmt.Sprintf(claudeEndpoint, c.Location, c.ProjectID, c.Location, c.Model)
}

// WithMaxRetries sets the maximum number of retries for the client


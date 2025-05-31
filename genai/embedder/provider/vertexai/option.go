package vertexai

import (
    "github.com/viant/agently/genai/embedder/provider/base"
    "github.com/viant/scy/auth/gcp"
    "net/http"
)

// ClientOption mutates a VertexAI Client instance.
type ClientOption func(*Client)

// Generic helpers reuse common implementation on embedded Config.
func WithBaseURL(baseURL string) ClientOption {
    return func(c *Client) { base.WithBaseURL(baseURL)(&c.Config) }
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
    return func(c *Client) { base.WithHTTPClient(httpClient)(&c.Config) }
}

func WithModel(model string) ClientOption {
    return func(c *Client) { base.WithModel(model)(&c.Config) }
}

// Vertex-specific options remain here.
func WithProjectID(projectID string) ClientOption {
    return func(c *Client) { c.ProjectID = projectID }
}

func WithLocation(location string) ClientOption {
    return func(c *Client) { c.Location = location }
}

func WithMaxRetries(max int) ClientOption {
    return func(c *Client) { c.MaxRetries = max }
}

func WithAuthService(svc *gcp.Service) ClientOption {
    return func(c *Client) { c.authService = svc }
}

func WithScopes(scopes ...string) ClientOption {
    return func(c *Client) { c.scopes = scopes }
}

func WithUsageListener(listener base.UsageListener) ClientOption {
    return func(c *Client) { c.UsageListener = listener }
}

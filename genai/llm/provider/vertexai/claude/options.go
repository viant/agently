package claude

import (
	"net/http"

	basecfg "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/scy/auth/gcp"
)

type ClientOption func(*Client)

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { basecfg.WithHTTPClient(httpClient)(&c.Config) }
}

func WithLocation(location string) ClientOption {
	return func(c *Client) { c.Location = location }
}

func WithProjectID(projectID string) ClientOption {
	return func(c *Client) { c.ProjectID = projectID }
}

func WithAnthropicVersion(version string) ClientOption {
	return func(c *Client) { c.AnthropicVersion = version }
}

func WithScopes(scopes ...string) ClientOption {
	return func(c *Client) { c.scopes = scopes }
}

func WithMaxRetries(max int) ClientOption {
	return func(c *Client) { c.MaxRetries = max }
}

func WithMaxTokens(max int) ClientOption {
	return func(c *Client) { c.MaxTokens = max }
}

func WithTemperature(temp float64) ClientOption {
	return func(c *Client) { c.Temperature = &temp }
}

// WithUsageListener registers a callback to receive token usage information.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.Config.UsageListener = l }
}

func WithAuthService(svc *gcp.Service) ClientOption {
	return func(c *Client) { c.authService = svc }
}

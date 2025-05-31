package claude

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// ClientOption is a function that configures a Client
type ClientOption func(*Client)

// WithAnthropicVersion sets the Anthropic API version for the client
func WithAnthropicVersion(version string) ClientOption {
	return func(c *Client) {
		c.AnthropicVersion = version
	}
}

// WithMaxRetries sets the maximum number of retries for the client
func WithMaxRetries(maxRetries int) ClientOption {
	return func(c *Client) {
		c.MaxRetries = maxRetries
	}
}

func WithConfig(config *aws.Config) ClientOption {
	return func(c *Client) {
		c.Config = config
	}
}

func WithRegion(region string) ClientOption {
	return func(c *Client) {
		c.Region = region
	}
}

func WithCredentialsURL(credentialsURL string) ClientOption {
	return func(c *Client) {
		c.CredentialsURL = credentialsURL
	}
}

// WithUsageListener registers a callback to receive token usage information.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
	return func(c *Client) { c.UsageListener = l }
}

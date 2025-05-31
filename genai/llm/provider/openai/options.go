package openai

import (
    "net/http"
    "time"

    basecfg "github.com/viant/agently/genai/llm/provider/base"
)


// ClientOption mutates an OpenAI Client instance.
type ClientOption func(*Client)

func WithBaseURL(baseURL string) ClientOption {
    return func(c *Client) { basecfg.WithBaseURL(baseURL)(&c.Config) }
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
    return func(c *Client) { basecfg.WithHTTPClient(httpClient)(&c.Config) }
}

func WithModel(model string) ClientOption {
    return func(c *Client) { basecfg.WithModel(model)(&c.Config) }
}

func WithTimeout(timeoutSeconds int) ClientOption {
    return func(c *Client) { basecfg.WithTimeout(time.Duration(timeoutSeconds) * time.Second)(&c.Config) }
}

// WithUsageListener assigns token usage listener to the client.
func WithUsageListener(l basecfg.UsageListener) ClientOption {
    return func(c *Client) { c.Config.UsageListener = l }
}

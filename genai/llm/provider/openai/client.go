package openai

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/viant/afs/storage"
	afsco "github.com/viant/afsc/openai"
	"github.com/viant/afsc/openai/assets"
	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

type APIKeyProvider func(ctx context.Context) (string, error)

// Client represents an OpenAI API client
type Client struct {
	basecfg.Config
	APIKey string
	// APIKeyProvider resolves the API key at call time (e.g., from OAuth token exchange).
	// When set, it is used only if APIKey is empty.
	APIKeyProvider APIKeyProvider
	// UserAgent overrides the default User-Agent header when specified and allowed.
	UserAgent string

	// Defaults applied when GenerateRequest.Options is nil or leaves the
	// respective field unset.
	MaxTokens        int
	Temperature      *float64
	storageMgr       storage.Manager
	storageMgrAPIKey string
	storageMgrMu     sync.Mutex

	// ContextContinuation controls whether this client should use
	// response continuation by response_id when supported. When nil,
	// continuation is considered enabled.
	ContextContinuation *bool
}

// NewClient creates a new OpenAI client with the given API key and model
func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: basecfg.Config{
			HTTPClient: &http.Client{Timeout: 30 * time.Minute}, // default; can be overridden
			BaseURL:    openAIEndpoint,
			Model:      model,
		},
		APIKey:     apiKey,
		storageMgr: nil,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	if client.APIKey == "" && client.APIKeyProvider == nil {
		client.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	// Optional: override HTTP timeout via environment variable (seconds)
	if v := os.Getenv("OPENAI_HTTP_TIMEOUT_SEC"); v != "" {
		if sec, err := time.ParseDuration(strings.TrimSpace(v) + "s"); err == nil && sec > 0 {
			client.Config.HTTPClient.Timeout = sec
			client.Config.Timeout = sec
		}
	}

	if client.APIKey != "" {
		client.storageMgrAPIKey = client.APIKey
		client.storageMgr = afsco.New(assets.NewConfig(client.APIKey))
	}

	return client
}

func (c *Client) apiKey(ctx context.Context) (string, error) {
	if c.APIKey != "" {
		return c.APIKey, nil
	}
	if c.APIKeyProvider == nil {
		return "", fmt.Errorf("API key is required")
	}
	key, err := c.APIKeyProvider(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("API key is required")
	}
	return key, nil
}

func (c *Client) userAgentOverride() string {
	if c == nil {
		return ""
	}
	ua := strings.TrimSpace(c.UserAgent)
	if ua == "" {
		return ""
	}
	low := strings.ToLower(ua)
	if strings.HasPrefix(low, "openai") || strings.HasPrefix(low, "open ai") {
		return ua
	}
	return ""
}

func (c *Client) ensureStorageManager(ctx context.Context) error {
	apiKey, err := c.apiKey(ctx)
	if err != nil {
		return err
	}
	c.storageMgrMu.Lock()
	defer c.storageMgrMu.Unlock()

	if c.storageMgr != nil && c.storageMgrAPIKey == apiKey {
		return nil
	}
	c.storageMgrAPIKey = apiKey
	c.storageMgr = afsco.New(assets.NewConfig(apiKey))
	return nil
}

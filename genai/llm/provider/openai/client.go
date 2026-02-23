package openai

import (
	"context"
	"fmt"
	"log"
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
type AuthDiagnosticsProvider func(ctx context.Context) string
type ChatGPTAccountIDProvider func(ctx context.Context) (string, error)

// Client represents an OpenAI API client
type Client struct {
	basecfg.Config
	APIKey string
	// APIKeyProvider resolves the API key at call time (e.g., from OAuth token exchange).
	// When set, it is used only if APIKey is empty.
	APIKeyProvider APIKeyProvider
	// UserAgent overrides the default User-Agent header when specified and allowed.
	UserAgent string
	// EnableLogging toggles provider runtime logs (auth/ws diagnostics).
	EnableLogging bool
	// Originator mirrors Codex default header style for backend-api compatibility.
	Originator string
	// CodexBetaFeatures maps to x-codex-beta-features header when set.
	CodexBetaFeatures string
	// AuthSource is a redacted label indicating where auth keys are resolved from.
	AuthSource string
	// AuthDiagnosticsProvider returns redacted runtime diagnostics for auth decisions.
	AuthDiagnosticsProvider AuthDiagnosticsProvider
	// ChatGPTAccountIDProvider resolves workspace/account id for ChatGPT backend requests.
	ChatGPTAccountIDProvider ChatGPTAccountIDProvider
	// ChatGPTAccountID is an optional static workspace/account id.
	ChatGPTAccountID string

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
		APIKey:        apiKey,
		EnableLogging: true,
		storageMgr:    nil,
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

func (c *Client) logf(format string, args ...interface{}) {
	if c == nil || !c.EnableLogging {
		return
	}
	log.Printf(format, args...)
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

func (c *Client) originatorHeader() string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.Originator); v != "" {
		return v
	}
	return "codex_cli_rs"
}

func (c *Client) codexBetaFeaturesHeader() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.CodexBetaFeatures)
}

func (c *Client) authDiagnostics(ctx context.Context) string {
	if c == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if src := strings.TrimSpace(c.AuthSource); src != "" {
		parts = append(parts, "auth_source="+src)
	}
	if c.AuthDiagnosticsProvider != nil {
		if extra := strings.TrimSpace(c.AuthDiagnosticsProvider(ctx)); extra != "" {
			parts = append(parts, extra)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (c *Client) chatGPTAccountID(ctx context.Context) (string, error) {
	if c == nil {
		return "", nil
	}
	if v := strings.TrimSpace(c.ChatGPTAccountID); v != "" {
		return v, nil
	}
	if c.ChatGPTAccountIDProvider == nil {
		return "", nil
	}
	v, err := c.ChatGPTAccountIDProvider(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

func isChatGPTBackendURL(baseURL string) bool {
	base := strings.ToLower(strings.TrimSpace(baseURL))
	if base == "" {
		return false
	}
	return strings.Contains(base, "chatgpt.com/backend-api") || strings.Contains(base, "chat.openai.com/backend-api")
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

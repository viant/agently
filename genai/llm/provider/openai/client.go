package openai

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/viant/afs/storage"
	afsco "github.com/viant/afsc/openai"
	"github.com/viant/afsc/openai/assets"
	basecfg "github.com/viant/agently/genai/llm/provider/base"
)

// Client represents an OpenAI API client
type Client struct {
	basecfg.Config
	APIKey string

	// Defaults applied when GenerateRequest.Options is nil or leaves the
	// respective field unset.
	MaxTokens   int
	Temperature *float64
	storageMgr  storage.Manager
}

// NewClient creates a new OpenAI client with the given API key and model
func NewClient(apiKey, model string, options ...ClientOption) *Client {
	client := &Client{
		Config: basecfg.Config{
			HTTPClient: &http.Client{Timeout: 15 * time.Minute}, // default; can be overridden
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

	if client.APIKey == "" {
		client.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	// Optional: override HTTP timeout via environment variable (seconds)
	if v := os.Getenv("OPENAI_HTTP_TIMEOUT_SEC"); v != "" {
		if sec, err := time.ParseDuration(strings.TrimSpace(v) + "s"); err == nil && sec > 0 {
			client.Config.HTTPClient.Timeout = sec
			client.Config.Timeout = sec
		}
	}

	client.storageMgr = afsco.New(assets.NewConfig(client.APIKey))

	return client
}

package mcp

// Client implements the mcp-protocol client-side Operations interface.  It is
// *not* a network client – instead it adapts protocol requests into local
// Agently capabilities (LLM generation, browser interaction, etc.).

import (
	"context"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/conv"
	"github.com/viant/jsonrpc"
	"github.com/viant/mcp-protocol/schema"
	"os/exec"
	"runtime"
)

// Client adapts MCP operations to local execution.
type Client struct {
	core       *core.Service
	openURLFn  func(string) error
	implements map[string]bool
}

func (c *Client) Init(ctx context.Context, capabilities *schema.ClientCapabilities) {
	if capabilities.Elicitation != nil {
		c.implements[schema.MethodElicitationCreate] = true
	}
	if capabilities.Roots != nil {
		c.implements[schema.MethodRootsList] = true
	}
	if capabilities.UserInteraction != nil {
		c.implements[schema.MethodInteractionCreate] = true
	}
	if capabilities.Sampling != nil {
		c.implements[schema.MethodSamplingCreateMessage] = true
	}
}

type Option func(*Client)

// -----------------------------------------------------------------------------
// Options
// -----------------------------------------------------------------------------

// WithLLMCore injects the llm/core service used to fulfil CreateMessage.
func WithLLMCore(svc *core.Service) Option {
	return func(c *Client) { c.core = svc }
}

// WithURLOpener overrides the function used to open browser.
func WithURLOpener(fn func(string) error) Option {
	return func(c *Client) { c.openURLFn = fn }
}

// -----------------------------------------------------------------------------
// Constructors & helpers
// -----------------------------------------------------------------------------

// NewClient returns a ready client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		openURLFn:  defaultOpenURL,
		implements: make(map[string]bool),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// defaultOpenURL tries best-effort to open the supplied URL in user browser.
func defaultOpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default: // linux, freebsd
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// -----------------------------------------------------------------------------
// Interface compliance helpers
// -----------------------------------------------------------------------------

func (c *Client) OnNotification(ctx context.Context, n *jsonrpc.Notification) {}

// Implements tells dispatcher which methods we support.
func (c *Client) Implements(method string) bool {
	switch method {
	case schema.MethodRootsList,
		schema.MethodSamplingCreateMessage,
		schema.MethodElicitationCreate,
		schema.MethodInteractionCreate:
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// Operations
// -----------------------------------------------------------------------------

func (c *Client) ListRoots(ctx context.Context, p *schema.ListRootsRequestParams) (*schema.ListRootsResult, *jsonrpc.Error) {
	// For local execution we have no workspace roots; return empty.
	return &schema.ListRootsResult{Roots: []schema.Root{}}, nil
}

func (c *Client) CreateUserInteraction(ctx context.Context, p *schema.CreateUserInteractionRequestParams) (*schema.CreateUserInteractionResult, *jsonrpc.Error) {
	if p == nil || p.Interaction.Url == "" {
		return nil, jsonrpc.NewInvalidParamsError("uri is required", nil)
	}
	_ = c.openURLFn(p.Interaction.Url) // ignore error – non-critical
	return &schema.CreateUserInteractionResult{}, nil
}

func (c *Client) Elicit(ctx context.Context, p *schema.ElicitRequestParams) (*schema.ElicitResult, *jsonrpc.Error) {
	// MVP: auto-decline; advanced UI can be added later.
	return &schema.ElicitResult{Action: schema.ElicitResultActionDecline}, nil
}

func (c *Client) CreateMessage(ctx context.Context, p *schema.CreateMessageRequestParams) (*schema.CreateMessageResult, *jsonrpc.Error) {
	if c.core == nil {
		return nil, jsonrpc.NewInternalError("llm core not configured", nil)
	}
	if p == nil {
		return nil, jsonrpc.NewInvalidParamsError("params is nil", nil)
	}

	// Use last message as prompt, earlier messages ignored in MVP.
	var prompt string
	if len(p.Messages) > 0 {
		prompt = p.Messages[len(p.Messages)-1].Content.Text // assuming text field
	}
	in := &core.GenerateInput{
		Preferences:  llm.NewModelPreferences(llm.WithPreferences(p.ModelPreferences)),
		Prompt:       prompt,
		SystemPrompt: conv.Dereference[string](p.SystemPrompt),
	}
	if p.MaxTokens > 0 || p.Temperature != nil || len(p.StopSequences) > 0 {
		in.Options = &llm.Options{
			MaxTokens:   p.MaxTokens,
			Temperature: conv.Dereference[float64](p.Temperature),
			StopWords:   p.StopSequences,
		}
	}

	var out core.GenerateOutput
	if err := c.core.Generate(ctx, in, &out); err != nil {
		return nil, jsonrpc.NewInternalError(err.Error(), nil)
	}

	result := &schema.CreateMessageResult{
		Model: in.Model,
		Role:  schema.RoleAssistant,
		Content: schema.CreateMessageResultContent{
			Type: "text",
			Text: out.Content,
		},
	}
	return result, nil
}

// -----------------------------------------------------------------------------
// Utility
// -----------------------------------------------------------------------------

func (c *Client) asErr(e error) *jsonrpc.Error {
	if e == nil {
		return nil
	}
	return jsonrpc.NewInternalError(e.Error(), nil)
}

// Option type remains in option.go

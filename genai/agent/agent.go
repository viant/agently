package agent

import (
	"fmt"
	"strings"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/embedius/matching/option"
)

type (

	// Identity represents actor identity

	Source struct {
		URL string `yaml:"url,omitempty" json:"url,omitempty"`
	}

	// ToolCallExposure controls how tool calls are exposed back to the LLM prompt
	// and templates. Supported modes:
	// - "turn": include only tool calls from the current turn
	// - "conversation": include tool calls from the whole conversation
	// - "semantic": reserved for future use (provider-native tool semantics)
	ToolCallExposure string

	// Agent represents an agent
	Agent struct {
		Identity `yaml:",inline" json:",inline"`
		Source   *Source `yaml:"source,omitempty" json:"source,omitempty"` // Source of the agent

		llm.ModelSelection `yaml:",inline" json:",inline"`

		Temperature float64        `yaml:"temperature,omitempty" json:"temperature,omitempty"` // Temperature
		Description string         `yaml:"description,omitempty" json:"description,omitempty"` // Description of the agent
		Prompt      *prompt.Prompt `yaml:"prompt,omitempty" json:"prompt,omitempty"`           // Prompt template
		Knowledge   []*Knowledge   `yaml:"knowledge,omitempty" json:"knowledge,omitempty"`

		// AutoSummarize controls whether the conversation is automatically
		// summarized/compacted after a turn (when supported by the runtime).
		AutoSummarize *bool `yaml:"autoSummarize,omitempty" json:"autoSummarize,omitempty"`

		// UI defaults: whether to show execution details and tool feed in chat
		ShowExecutionDetails *bool `yaml:"showExecutionDetails,omitempty" json:"showExecutionDetails,omitempty"`
		ShowToolFeed         *bool `yaml:"showToolFeed,omitempty" json:"showToolFeed,omitempty"`

		// RingOnFinish enables a short client-side notification sound when a turn
		// completes (done or error). Consumed by the UI via metadata.AgentInfo.
		RingOnFinish bool `yaml:"ringOnFinish,omitempty" json:"ringOnFinish,omitempty"`

		SystemPrompt    *prompt.Prompt `yaml:"systemPrompt,omitempty" json:"systemPrompt,omitempty"`
		SystemKnowledge []*Knowledge   `yaml:"systemKnowledge,omitempty" json:"systemKnowledge,omitempty"`
		Tool            []*llm.Tool    `yaml:"tool,omitempty" json:"tool,omitempty"`

		// ParallelToolCalls requests providers that support it to execute
		// multiple tool calls in parallel within a single reasoning step.
		// Honored only when the selected model implements the feature.
		ParallelToolCalls bool `yaml:"parallelToolCalls,omitempty" json:"parallelToolCalls,omitempty"`

		// ToolCallExposure defines how tool calls are exposed to the LLM
		ToolCallExposure ToolCallExposure `yaml:"toolCallExposure,omitempty" json:"toolCallExposure,omitempty"`

		// Persona defines the default conversational persona the agent uses when
		// sending messages. When nil the role defaults to "assistant".
		Persona *prompt.Persona `yaml:"persona,omitempty" json:"persona,omitempty"`

		// Profile controls agent discoverability in the catalog/list (preferred over Directory).
		Profile *Profile `yaml:"profile,omitempty" json:"profile,omitempty"`

		// Serve groups serving endpoints (e.g., A2A). Preferred over legacy ExposeA2A.
		Serve *Serve `yaml:"serve,omitempty" json:"serve,omitempty"`

		// ExposeA2A (legacy) controls optional exposure of this agent as an external A2A server
		ExposeA2A *ExposeA2A `yaml:"exposeA2A,omitempty" json:"exposeA2A,omitempty"`

		// Attachment groups binary-attachment behavior
		Attachment *Attachment `yaml:"attachment,omitempty" json:"attachment,omitempty"`

		// Chains defines post-turn follow-ups executed after a turn finishes.
		Chains []*Chain `yaml:"chains,omitempty" json:"chains,omitempty"`

		// MCPResources controls optional inclusion of MCP-accessible resources
		// as binding documents. When enabled, the agent service lazily indexes
		// the specified locations with Embedius, selects the top-N most relevant
		// resources for the current query, and includes their content in the
		// binding Documents so prompts/templates can reason over them directly.
		MCPResources *MCPResources `yaml:"mcpResources,omitempty" json:"mcpResources,omitempty"`
	}

	// MCPResources defines matching and selection rules for attaching resources
	// discovered via MCP (or generic locations) to the LLM request.
	MCPResources struct {
		Enabled   bool            `yaml:"enabled,omitempty" json:"enabled,omitempty"`
		Locations []string        `yaml:"locations,omitempty" json:"locations,omitempty"`
		MaxFiles  int             `yaml:"maxFiles,omitempty" json:"maxFiles,omitempty"`
		TrimPath  string          `yaml:"trimPath,omitempty" json:"trimPath,omitempty"`
		Match     *option.Options `yaml:"match,omitempty" json:"match,omitempty"`
		MinScore  *float64        `yaml:"minScore,omitempty" json:"minScore,omitempty"`
	}

	// Chain defines a single post-turn follow-up.
	Chain struct {
		On           string      `yaml:"on,omitempty" json:"on,omitempty"`                     // succeeded|failed|canceled|*
		Target       ChainTarget `yaml:"target" json:"target"`                                 // required: agent to invoke
		Conversation string      `yaml:"conversation,omitempty" json:"conversation,omitempty"` // reuse|link (default link)
		When         *WhenSpec   `yaml:"when,omitempty" json:"when,omitempty"`                 // optional condition

		Query *prompt.Prompt `yaml:"query,omitempty" json:"query,omitempty"` // templated query/payload

		Publish *ChainPublish `yaml:"publish,omitempty" json:"publish,omitempty"` // optional publish settings
		OnError string        `yaml:"onError,omitempty" json:"onError,omitempty"` // ignore|message|propagate
		Limits  *ChainLimits  `yaml:"limits,omitempty" json:"limits,omitempty"`   // guard-rails
	}

	ChainTarget struct {
		AgentID string `yaml:"agentId" json:"agentId"`
	}

	ChainPublish struct {
		Role   string `yaml:"role,omitempty" json:"role,omitempty"`     // assistant|user|system|tool|none
		Name   string `yaml:"name,omitempty" json:"name,omitempty"`     // attribution handle
		Type   string `yaml:"type,omitempty" json:"type,omitempty"`     // text|control
		Parent string `yaml:"parent,omitempty" json:"parent,omitempty"` // same_turn|last_user|none
	}

	ChainLimits struct {
		MaxDepth int `yaml:"maxDepth,omitempty" json:"maxDepth,omitempty"`
	}
)

type Tool struct {
	Items              []*llm.Tool      `yaml:"items,omitempty" json:"items,omitempty"`
	ResultPreviewLimit *int             `yaml:"resultPreviewLimit,omitempty" json:"resultPreviewLimit,omitempty"`
	CallExposure       ToolCallExposure `yaml:"toolCallExposure,omitempty" json:"toolCallExposure,omitempty"`
}

// Directory (legacy) removed – use Profile.

// Profile controls discoverability in the agent catalog/list.
type Profile struct {
	Publish     bool     `yaml:"publish,omitempty" json:"publish,omitempty"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Rank        int      `yaml:"rank,omitempty" json:"rank,omitempty"`
	// Future-proof: extra metadata for presentation
	Capabilities     map[string]interface{} `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Responsibilities []string               `yaml:"responsibilities,omitempty" json:"responsibilities,omitempty"`
	InScope          []string               `yaml:"inScope,omitempty" json:"inScope,omitempty"`
	OutOfScope       []string               `yaml:"outOfScope,omitempty" json:"outOfScope,omitempty"`
}

// ExposeA2A (legacy): retained for backward compatibility; use Serve.A2A instead.
type ExposeA2A struct {
	Enabled   bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Port      int      `yaml:"port,omitempty" json:"port,omitempty"`
	BasePath  string   `yaml:"basePath,omitempty" json:"basePath,omitempty"`
	Streaming bool     `yaml:"streaming,omitempty" json:"streaming,omitempty"`
	Auth      *A2AAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// Serve groups serving endpoints for this agent (e.g., A2A).
type Serve struct {
	A2A *ServeA2A `yaml:"a2a,omitempty" json:"a2a,omitempty"`
}

// ServeA2A declares how to expose an internal agent as an A2A server.
type ServeA2A struct {
	Enabled   bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Port      int      `yaml:"port,omitempty" json:"port,omitempty"`
	Streaming bool     `yaml:"streaming,omitempty" json:"streaming,omitempty"`
	Auth      *A2AAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// A2AAuth configures per-agent A2A auth middleware.
type A2AAuth struct {
	Enabled       bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Resource      string   `yaml:"resource,omitempty" json:"resource,omitempty"`
	Scopes        []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	UseIDToken    bool     `yaml:"useIDToken,omitempty" json:"useIDToken,omitempty"`
	ExcludePrefix string   `yaml:"excludePrefix,omitempty" json:"excludePrefix,omitempty"`
}

// Attachment configures binary attachment behavior for an agent.
type Attachment struct {
	// LimitBytes caps cumulative attachments size per conversation for this agent.
	// When zero, a provider default may apply or no cap if provider has none.
	LimitBytes int64 `yaml:"limitBytes,omitempty" json:"limitBytes,omitempty"`

	// Mode controls delivery: "ref" or "inline"
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// TTLSec sets TTL for attachments in seconds.
	TTLSec int64 `yaml:"ttlSec,omitempty" json:"ttlSec,omitempty"`
}

// Init applies default values to the agent after it has been loaded from YAML.
// It should be invoked by the loader to ensure a single place for defaults.
func (a *Agent) Init() {
	if a == nil {
		return
	}
	// Ensure attachment block exists with sane defaults
	if a.Attachment == nil {
		a.Attachment = &Attachment{}
	}
	if a.Attachment.Mode == "" {
		a.Attachment.Mode = "ref"
	}
	// Defaults for UI flags – default to true when unspecified
	if a.ShowExecutionDetails == nil {
		v := true
		a.ShowExecutionDetails = &v
	}
	if a.ShowToolFeed == nil {
		v := true
		a.ShowToolFeed = &v
	}
}

// WhenSpec specifies a conditional gate for executing a chain. Evaluate Expr first; if empty and Query present,
// run an LLM prompt and extract a boolean using Expect.
type WhenSpec struct {
	Expr   string         `yaml:"expr,omitempty" json:"expr,omitempty"`
	Query  *prompt.Prompt `yaml:"query,omitempty" json:"query,omitempty"`
	Model  string         `yaml:"model,omitempty" json:"model,omitempty"`
	Expect *WhenExpect    `yaml:"expect,omitempty" json:"expect,omitempty"`
}

// WhenExpect describes how to extract a boolean from an LLM response.
// Supported kinds: boolean (default), regex, jsonpath (basic $.field).
type WhenExpect struct {
	Kind    string `yaml:"kind,omitempty" json:"kind,omitempty"`
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
}

func (a *Agent) Validate() error {
	if a == nil {
		return fmt.Errorf("agent is nil")
	}
	// Validate chains: target.agentId must be non-empty when chains are declared
	for i, c := range a.Chains {
		if c == nil {
			continue
		}
		if strings.TrimSpace(c.Target.AgentID) == "" {
			return fmt.Errorf("invalid chain[%d]: target.agentId is required", i)
		}
		if conv := strings.ToLower(strings.TrimSpace(c.Conversation)); conv != "" && conv != "reuse" && conv != "link" {
			return fmt.Errorf("invalid chain[%d]: conversation must be reuse or link", i)
		}
	}
	return nil
}

func (a *Agent) HasAutoSummarizeDefinition() bool {
	return a.AutoSummarize != nil
}

func (a *Agent) ShallAutoSummarize() bool {
	if a.AutoSummarize == nil {
		return false
	}
	return *a.AutoSummarize
}

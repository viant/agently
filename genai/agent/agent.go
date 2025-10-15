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

		// ToolExport controls automatic exposure of this agent as a virtual tool
		ToolExport *ToolExport `yaml:"toolExport,omitempty" json:"toolExport,omitempty"`

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
	}

	// ToolExport defines optional settings to expose an agent as a runtime tool.
	ToolExport struct {
		Expose  bool     `yaml:"expose,omitempty" json:"expose,omitempty"`   // opt-in flag
		Service string   `yaml:"service,omitempty" json:"service,omitempty"` // MCP service name (default "agentExec")
		Method  string   `yaml:"method,omitempty" json:"method,omitempty"`   // Method name (default agent.id)
		Domains []string `yaml:"domains,omitempty" json:"domains,omitempty"` // Allowed parent domains
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

// Attachment configures binary attachment behavior for an agent.
type Attachment struct {
	// LimitBytes caps cumulative attachments size per conversation for this agent.
	// When zero, a provider default may apply or no cap if provider has none.
	LimitBytes int64 `yaml:"limitBytes,omitempty" json:"limitBytes,omitempty"`

	// Mode controls delivery: "ref" or "inline"
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// TTLSec sets TTL for attachments in seconds.
	TTLSec int64 `yaml:"ttlSec,omitempty" json:"ttlSec,omitempty"`

	// ToolCallConversionThreshold sets default threshold (bytes) to convert
	// tool call results into PDF attachments (provider-dependent).
	ToolCallConversionThreshold int64 `yaml:"toolCallConversionThreshold,omitempty" json:"toolCallConversionThreshold,omitempty"`
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
	if a.Attachment.ToolCallConversionThreshold <= 0 {
		// Default 100k threshold for tool-call → PDF conversion
		a.Attachment.ToolCallConversionThreshold = 100_000
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

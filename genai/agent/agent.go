package agent

import (
	"fmt"
	"strings"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
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

		// AttachmentLimitBytes caps cumulative attachments size per conversation for this agent.
		// When zero, a provider default may apply (e.g., OpenAI 32MiB) or no cap if provider has none.
		AttachmentLimitBytes int64  `yaml:"attachmentLimitBytes,omitempty" json:"attachmentLimitBytes,omitempty"`
		AttachMode           string `yaml:"attachMode,omitempty" json:"attachMode,omitempty"` // "ref" | "inline"
		AttachmentTTLSec     int64  `yaml:"attachmentTTLSec,omitempty" json:"attachmentTTLSec,omitempty"`

		// Chains defines post-turn follow-ups executed after a turn finishes.
		Chains []*Chain `yaml:"chains,omitempty" json:"chains,omitempty"`
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
		On           string         `yaml:"on,omitempty" json:"on,omitempty"`                     // succeeded|failed|canceled|*
		Target       ChainTarget    `yaml:"target" json:"target"`                                 // required: agent to invoke
		Mode         string         `yaml:"mode,omitempty" json:"mode,omitempty"`                 // async|sync|queue (default sync)
		Conversation string         `yaml:"conversation,omitempty" json:"conversation,omitempty"` // reuse|link (default link)
		Query        *prompt.Prompt `yaml:"query,omitempty" json:"query,omitempty"`               // templated query/payload
		When         string         `yaml:"when,omitempty" json:"when,omitempty"`                 // optional condition
		Publish      *ChainPublish  `yaml:"publish,omitempty" json:"publish,omitempty"`           // optional publish settings
		OnError      string         `yaml:"onError,omitempty" json:"onError,omitempty"`           // ignore|message|propagate
		Limits       *ChainLimits   `yaml:"limits,omitempty" json:"limits,omitempty"`             // guard-rails
	}

	ChainTarget struct {
		AgentID string `yaml:"agentId" json:"agentId"`
	}

	ChainPublish struct {
		Role         string `yaml:"role,omitempty" json:"role,omitempty"`                 // assistant|user|system|tool|none
		Name         string `yaml:"name,omitempty" json:"name,omitempty"`                 // attribution handle
		Type         string `yaml:"type,omitempty" json:"type,omitempty"`                 // text|control
		Parent       string `yaml:"parent,omitempty" json:"parent,omitempty"`             // same_turn|last_user|none
		AutoNextTurn bool   `yaml:"autoNextTurn,omitempty" json:"autoNextTurn,omitempty"` // only with role=user
	}

	ChainLimits struct {
		MaxDepth  int    `yaml:"maxDepth,omitempty" json:"maxDepth,omitempty"`
		DedupeKey string `yaml:"dedupeKey,omitempty" json:"dedupeKey,omitempty"`
	}
)

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
		if m := strings.ToLower(strings.TrimSpace(c.Mode)); m != "" && m != "sync" && m != "async" {
			return fmt.Errorf("invalid chain[%d]: mode must be sync or async", i)
		}
		if conv := strings.ToLower(strings.TrimSpace(c.Conversation)); conv != "" && conv != "reuse" && conv != "link" {
			return fmt.Errorf("invalid chain[%d]: conversation must be reuse or link", i)
		}
	}
	return nil
}

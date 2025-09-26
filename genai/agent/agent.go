package agent

import (
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
)

type (

	// Identity represents actor identity

	Source struct {
		URL string `yaml:"url,omitempty" json:"url,omitempty"`
	}

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

		// Elicitation optionally defines required context schema that must be
		// satisfied before the agent can execute its workflow. When provided, the
		// runtime checks incoming QueryInput.Context against the schema and, if
		// required properties are missing, responds with an elicitation request
		// to gather the missing data from the caller.
		Elicitation *plan.Elicitation `yaml:"elicitation,omitempty" json:"elicitation,omitempty"`

		// Persona defines the default conversational persona the agent uses when
		// sending messages. When nil the role defaults to "assistant".
		Persona *prompt.Persona `yaml:"persona,omitempty" json:"persona,omitempty"`

		// ToolExport controls automatic exposure of this agent as a virtual tool
		ToolExport *ToolExport `yaml:"toolExport,omitempty" json:"toolExport,omitempty"`
	}

	// ToolExport defines optional settings to expose an agent as a runtime tool.
	ToolExport struct {
		Expose  bool     `yaml:"expose,omitempty" json:"expose,omitempty"`   // opt-in flag
		Service string   `yaml:"service,omitempty" json:"service,omitempty"` // MCP service name (default "agentExec")
		Method  string   `yaml:"method,omitempty" json:"method,omitempty"`   // Method name (default agent.id)
		Domains []string `yaml:"domains,omitempty" json:"domains,omitempty"` // Allowed parent domains
	}
)

func (a *Agent) Validate() error {
	return nil
}

package agent

import (
	"bytes"
	"sync"
	"text/template"

	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/internal/templating"
)

type (

	// Identity represents actor identity

	Source struct {
		URL string `yaml:"url,omitempty" json:"url,omitempty"`
	}

	// Agent represents an agent
	Agent struct {
		Identity    `yaml:",inline" json:",inline"`
		Source      *Source `yaml:"source,omitempty" json:"source,omitempty"`           // Source of the agent
		Model       string  `yaml:"modelRef,omitempty" json:"model,omitempty"`          // Language model configuration
		Temperature float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"` // Temperature
		Description string  `yaml:"description,omitempty" json:"description,omitempty"` // Description of the agent
		Prompt      string  `yaml:"prompt,omitempty" json:"prompt,omitempty"`           // Prompt template

		Tool []*llm.Tool `yaml:"tool,omitempty" json:"tool,omitempty"`

		// Agent's knowledge base (optional)
		Knowledge       []*Knowledge `yaml:"knowledge,omitempty" json:"knowledge,omitempty"`
		SystemKnowledge []*Knowledge `yaml:"systemKnowledge,omitempty" json:"systemKnowledge,omitempty"`

		// OrchestrationFlow optional path/URL to override default workflow graph.
		OrchestrationFlow string `yaml:"orchestrationFlow,omitempty" json:"orchestrationFlow,omitempty"`

		// Elicitation optionally defines required context schema that must be
		// satisfied before the agent can execute its workflow. When provided, the
		// runtime checks incoming QueryInput.Context against the schema and, if
		// required properties are missing, responds with an elicitation request
		// to gather the missing data from the caller.
		Elicitation *plan.Elicitation `yaml:"elicitation,omitempty" json:"elicitation,omitempty"`

		// Persona defines the default conversational persona the agent uses when
		// sending messages. When nil the role defaults to "assistant".
		Persona *Persona `yaml:"persona,omitempty" json:"persona,omitempty"`

		// ToolExport controls automatic exposure of this agent as a virtual tool
		ToolExport *ToolExport `yaml:"toolExport,omitempty" json:"toolExport,omitempty"`

		// cached compiled go template for prompt (if Prompt is static)
		parsedTemplate *template.Template `yaml:"-" json:"-"`
		once           sync.Once          `yaml:"-" json:"-"`
		parseErr       error              `yaml:"-" json:"-"`
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

// GeneratePrompt generates a prompt from the agent's template using provided query and enrichment data
func (a *Agent) GeneratePrompt(query string, enrichment string) (string, error) {
	if a.Prompt == "" {
		// Use default template if not specified
		return a.generateDefaultPrompt(query, enrichment), nil
	}

	// Try to use velty template engine first
	promptText, err := a.generateVeltyPrompt(query, enrichment)
	if err == nil {
		return promptText, nil
	}

	// Fall back to text/template if velty fails
	return a.generateGoTemplatePrompt(query, enrichment)
}

// generateVeltyPrompt uses velty engine to process the template
func (a *Agent) generateVeltyPrompt(query string, enrichment string) (string, error) {
	vars := map[string]interface{}{
		"Find":       a,
		"Query":      query,
		"Enrichment": enrichment,
	}
	return templating.Expand(a.Prompt, vars)
}

// generateGoTemplatePrompt uses Go's text/template to process the template
func (a *Agent) generateGoTemplatePrompt(query string, enrichment string) (string, error) {
	// lazily compile template once
	a.once.Do(func() {
		a.parsedTemplate, a.parseErr = template.New("prompt").Parse(a.Prompt)
	})
	if a.parseErr != nil {
		return "", a.parseErr
	}

	data := map[string]interface{}{
		"Find":       a,
		"Query":      query,
		"Enrichment": enrichment,
	}

	var buf bytes.Buffer
	if err := a.parsedTemplate.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// generateDefaultPrompt creates a simple default prompt if no template is provided
func (a *Agent) generateDefaultPrompt(query string, enrichment string) string {
	var buf bytes.Buffer

	buf.WriteString("You are ")
	if a.Name != "" {
		buf.WriteString(a.Name)
	} else {
		buf.WriteString("an AI assistant")
	}

	if a.Description != "" {
		buf.WriteString(", ")
		buf.WriteString(a.Description)
	}

	buf.WriteString("\n\n")

	if enrichment != "" {
		buf.WriteString("Document details:\n")
		buf.WriteString(enrichment)
		buf.WriteString("\n\n")
	}

	return buf.String()
}

package prompt

import (
	"github.com/viant/agently/genai/llm"
)

type (
	Flags struct {
		CanUseTool bool `yaml:"canUseTool,omitempty" json:"canUseTool,omitempty"`
		IsSystem   bool `yaml:"isSystemPath,omitempty" json:"isSystemPath,omitempty"`
	}

	Documents struct {
		Items []*Document `yaml:"items,omitempty" json:"items,omitempty"`
	}

	Document struct {
		Title       string            `yaml:"title,omitempty" json:"title,omitempty"`
		PageContent string            `yaml:"pageContent,omitempty" json:"pageContent,omitempty"`
		SourceURI   string            `yaml:"sourceURI,omitempty" json:"sourceURI,omitempty"`
		Score       float64           `yaml:"score,omitempty" json:"score,omitempty"`
		MimeType    string            `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`
		Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	}

	Tools struct {
		Signatures []*llm.ToolDefinition `yaml:"signatures,omitempty" json:"signatures,omitempty"`
		Executions []*llm.ToolCall       `yaml:"executions,omitempty" json:"executions,omitempty"`
	}

	Message struct {
		Role     string `yaml:"role,omitempty" json:"role,omitempty"`
		MimeType string `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`
		Content  string `yaml:"content,omitempty" json:"content,omitempty"`
	}

	History struct {
		Messages []*Message `yaml:"messages,omitempty" json:"messages,omitempty"`
	}

	Task struct {
		Prompt string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	}

	Binding struct {
		Task            Task                   `yaml:"task" json:"task"`
		Persona         *Persona               `yaml:"persona,omitempty" json:"persona,omitempty"`
		History         History                `yaml:"history,omitempty" json:"history,omitempty"`
		Tools           *Tools                 `yaml:"tools,omitempty" json:"tools,omitempty"`
		SystemDocuments Documents              `yaml:"systemDocuments,omitempty" json:"systemDocuments,omitempty"`
		Documents       Documents              `yaml:"documents,omitempty" json:"documents,omitempty"`
		Flags           Flags                  `yaml:"flags,omitempty" json:"flags,omitempty"`
		Context         map[string]interface{} `yaml:"context,omitempty" json:"context,omitempty"`
	}
)

func (b *Binding) SystemBinding() *Binding {
	clone := *b
	clone.Flags.IsSystem = true
	return &clone
}

func (b *Binding) Data() map[string]interface{} {
	var context = map[string]interface{}{
		"Task":          b.Task,
		"History":       b.History,
		"Tools":         b.Tools,
		"Flags":         b.Flags,
		"LoadDocuments": b.Documents,
		"Context":       b.Context,
	}
	return context
}

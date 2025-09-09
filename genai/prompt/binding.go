package prompt

import "encoding/json"

type (
	Flags struct {
		CanUseTool bool `yaml:"canUseTool,omitempty" json:"canUseTool,omitempty"`
		IsSystem   bool `yaml:"isSystemPath,omitempty" json:"isSystemPath,omitempty"`
	}

	Documents struct {
		Items []*Document `yaml:"items,omitempty" json:"items,omitempty"`
	}

	Document struct {
		Title     string            `yaml:"title,omitempty" json:"title,omitempty"`
		Snippet   string            `yaml:"snippet,omitempty" json:"snippet,omitempty"`
		SourceURI string            `yaml:"sourceURI,omitempty" json:"sourceURI,omitempty"`
		Score     float64           `yaml:"score,omitempty" json:"score,omitempty"`
		MimeType  string            `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`
		Metadata  map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	}

	ToolDefinition struct {
		Name         string          `yaml:"name,omitempty" json:"name,omitempty"`
		Description  string          `yaml:"description,omitempty" json:"description,omitempty"`
		InputSchema  json.RawMessage `yaml:"inputSchema,omitempty" json:"inputSchema,omitempty"`
		OutputSchema json.RawMessage `yaml:"outputSchema,omitempty" json:"outputSchema,omitempty"`
	}

	ToolCall struct {
		Name          string `yaml:"name,omitempty" json:"name,omitempty"`
		Status        string `yaml:"status,omitempty" json:"status,omitempty"`
		ResultSummary string `yaml:"resultSummary,omitempty" json:"resultSummary,omitempty"`
		Error         string `yaml:"error,omitempty" json:"error,omitempty"`
		Elapsed       string `yaml:"elapsed,omitempty" json:"elapsed,omitempty"`
	}

	Tools struct {
		Signatures []*ToolDefinition `yaml:"signatures,omitempty" json:"signatures,omitempty"`
		Executions []*ToolCall       `yaml:"executions,omitempty" json:"executions,omitempty"`
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
		UserPrompt string `yaml:"userPrompt,omitempty" json:"userPrompt,omitempty"`
	}

	Binding struct {
		Task      Task      `yaml:"task" json:"task"`
		History   History   `yaml:"history,omitempty" json:"history,omitempty"`
		Tools     Tools     `yaml:"tools,omitempty" json:"tools,omitempty"`
		Documents Documents `yaml:"documents,omitempty" json:"documents,omitempty"`
		Flags     Flags     `yaml:"flags,omitempty" json:"flags,omitempty"`
	}
)

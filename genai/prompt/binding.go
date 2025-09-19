package prompt

import (
	"path"
	"strings"

	"github.com/viant/agently/genai/llm"
)

type (
	Flags struct {
		CanUseTool   bool `yaml:"canUseTool,omitempty" json:"canUseTool,omitempty"`
		CanStream    bool `yaml:"canStream,omitempty" json:"canStream,omitempty"`
		IsMultimodal bool `yaml:"isMultimodal,omitempty" json:"isMultimodal,omitempty"`
		IsSystem     bool `yaml:"isSystemPath,omitempty" json:"isSystemPath,omitempty"`
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
		Role       string        `yaml:"role,omitempty" json:"role,omitempty"`
		MimeType   string        `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`
		Content    string        `yaml:"content,omitempty" json:"content,omitempty"`
		Attachment []*Attachment `yaml:"attachment,omitempty" json:"attachment,omitempty"`
	}

	Attachment struct {
		Name          string `yaml:"name,omitempty" json:"name,omitempty"`
		URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`
		StagingFolder string `yaml:"stagingFolder,omitempty" json:"stagingFolder,omitempty"`
		Mime          string `yaml:"mime,omitempty" json:"mime,omitempty"`
		Content       string `yaml:"content,omitempty" json:"content,omitempty"`
		Data          []byte `yaml:"data,omitempty" json:"data,omitempty"`
	}

	History struct {
		Messages []*Message `yaml:"messages,omitempty" json:"messages,omitempty"`
	}

	Task struct {
		Prompt      string        `yaml:"prompt,omitempty" json:"prompt,omitempty"`
		Attachments []*Attachment `yaml:"attachments,omitempty" json:"attachments,omitempty"`
	}

	Meta struct {
		Model string `yaml:"model,omitempty" json:"model,omitempty"`
	}

	Binding struct {
		Task            Task                   `yaml:"task" json:"task"`
		Persona         Persona                `yaml:"persona,omitempty" json:"persona,omitempty"`
		History         History                `yaml:"history,omitempty" json:"history,omitempty"`
		Tools           Tools                  `yaml:"tools,omitempty" json:"tools,omitempty"`
		Meta            Meta                   `yaml:"meta,omitempty" json:"meta,omitempty"`
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
		"Task":            &b.Task,
		"History":         &b.History,
		"Tools":           &b.Tools,
		"Flags":           &b.Flags,
		"Documents":       &b.Documents,
		"Meta":            &b.Meta,
		"Context":         &b.Context,
		"SystemDocuments": &b.SystemDocuments,
	}

	// Flatten selected keys from Context into top-level for convenience
	for k, v := range b.Context {
		if _, exists := context[k]; !exists {
			context[k] = v
		}
	}
	return context
}

func (a *Attachment) Type() string {
	mimeType := a.MIMEType()
	if index := strings.LastIndex(mimeType, "/"); index != -1 {
		mimeType = mimeType[index+1:]
	}
	return mimeType
}

func (a *Attachment) MIMEType() string {
	if a.Mime != "" {
		return a.Mime
	}
	// Handle empty Name case
	if a.Name == "" {
		return "application/octet-Stream"
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(a.Name), "."))
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "pdf":
		return "application/pdf"
	case "txt":
		return "text/plain"
	case "md":
		return "text/markdown"
	case "csv":
		return "text/csv"
	case "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "html":
		return "text/html"
	case "yaml", "yml":
		return "application/x-yaml"
	case "zip":
		return "application/zip"
	case "tar":
		return "application/x-tar"
	case "mp3":
		return "audio/mpeg"
	case "mp4":
		return "video/mp4"
	}
	return "application/octet-Stream"
}

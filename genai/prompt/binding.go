package prompt

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
)

type (
	Flags struct {
		CanUseTool   bool `yaml:"canUseTool,omitempty" json:"canUseTool,omitempty"`
		CanStream    bool `yaml:"canStream,omitempty" json:"canStream,omitempty"`
		IsMultimodal bool `yaml:"isMultimodal,omitempty" json:"isMultimodal,omitempty"`
		IsSystem     bool `yaml:"isSystemPath,omitempty" json:"isSystemPath,omitempty"`
		// HasMessageOverflow indicates that a message content (tool result or otherwise)
		// exceeded the preview limit and the binding may expose message helpers.
		HasMessageOverflow bool `yaml:"hasMessageOverflow,omitempty" json:"hasMessageOverflow,omitempty"`
		// MaxOverflowBytes records the maximum original byte size of any
		// message that triggered overflow in this binding (history or tool
		// results). When zero, no size information was recorded.
		MaxOverflowBytes int `yaml:"maxOverflowBytes,omitempty" json:"maxOverflowBytes,omitempty"`
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
		Messages        []*Message `yaml:"messages,omitempty" json:"messages,omitempty"`
		UserElicitation []*Message `yaml:"userElicitation" json:"userElicitation"`
		LastResponse    *Trace
		Traces          map[string]*Trace
	}
)

type Kind string

const (
	KindResponse Kind = "resp"
	KindToolCall Kind = "op"
	KindContent  Kind = "content"
)

type (
	Task struct {
		Prompt      string        `yaml:"prompt,omitempty" json:"prompt,omitempty"`
		Attachments []*Attachment `yaml:"attachments,omitempty" json:"attachments,omitempty"`
	}

	Meta struct {
		Model string `yaml:"model,omitempty" json:"model,omitempty"`
	}

	Binding struct {
		Task            Task                   `yaml:"task" json:"task"`
		Model           string                 `yaml:"model,omitempty" json:"model,omitempty"`
		Persona         Persona                `yaml:"persona,omitempty" json:"persona,omitempty"`
		History         History                `yaml:"history,omitempty" json:"history,omitempty"`
		Tools           Tools                  `yaml:"tools,omitempty" json:"tools,omitempty"`
		Meta            Meta                   `yaml:"meta,omitempty" json:"meta,omitempty"`
		SystemDocuments Documents              `yaml:"systemDocuments,omitempty" json:"systemDocuments,omitempty"`
		Documents       Documents              `yaml:"documents,omitempty" json:"documents,omitempty"`
		Flags           Flags                  `yaml:"flags,omitempty" json:"flags,omitempty"`
		Context         map[string]interface{} `yaml:"context,omitempty" json:"context,omitempty"`
		// Elicitation contains a generic, prompt-friendly view of agent-required inputs
		// so templates can instruct the LLM to elicit missing data when necessary.
		Elicitation Elicitation `yaml:"elicitation,omitempty" json:"elicitation,omitempty"`
	}

	// Elicitation is a generic holder for required-input prompts used by templates.
	// It intentionally avoids coupling to agent plan types.
	Elicitation struct {
		Required bool                   `yaml:"required,omitempty" json:"required,omitempty"`
		Missing  []string               `yaml:"missing,omitempty" json:"missing,omitempty"`
		Message  string                 `yaml:"message,omitempty" json:"message,omitempty"`
		Schema   map[string]interface{} `yaml:"schema,omitempty" json:"schema,omitempty"`
		// SchemaJSON is a pre-serialized JSON of Schema for templates without JSON helpers
		SchemaJSON string `yaml:"schemaJSON,omitempty" json:"schemaJSON,omitempty"`
	}

	Trace struct {
		ID   string
		Kind Kind
		At   time.Time
	}
)

// IsValid reports whether the trace carries a usable anchor.
// A valid anchor must have a non-zero time; ID may be empty when
// provider continuation by response id is not used.
func (t *Trace) IsValid() bool {
	return t != nil && !t.At.IsZero()
}

// Kind helpers
func (k Kind) IsToolCall() bool {
	return strings.EqualFold(strings.TrimSpace(string(k)), string(KindToolCall))
}
func (k Kind) IsResponse() bool {
	return strings.EqualFold(strings.TrimSpace(string(k)), string(KindResponse))
}
func (k Kind) IsContent() bool {
	return strings.EqualFold(strings.TrimSpace(string(k)), string(KindContent))
}

// Key produces a stable map key for the given raw value based on the Kind.
// - For KindResponse and KindToolCall, it trims whitespace and returns the id/opId.
// - For KindContent, it derives a hash key from normalized content.
func (k Kind) Key(raw string) string {
	switch {
	case k.IsContent():
		return "content:" + MakeContentKey(raw)
	case k.IsToolCall():
		return "tool:" + strings.TrimSpace(raw)
	case k.IsResponse():
		return "resp:" + strings.TrimSpace(raw)
	default:
		return strings.TrimSpace(raw)
	}
}

// NormalizeContent trims whitespace and, if content is valid JSON, returns
// its minified canonical form.
func NormalizeContent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	var tmp interface{}
	if json.Unmarshal([]byte(s), &tmp) == nil {
		if b, err := json.Marshal(tmp); err == nil {
			return string(b)
		}
	}
	return s
}

// MakeContentKey builds a stable key for text contents by hashing normalized content.
func MakeContentKey(content string) string {
	norm := NormalizeContent(content)
	h := sha1.Sum([]byte(norm))
	return hex.EncodeToString(h[:])
}

func (b *Binding) SystemBinding() *Binding {
	clone := *b
	clone.Flags.IsSystem = true
	return &clone
}

func (b *Binding) Data() map[string]interface{} {
	var context = map[string]interface{}{
		"Task":            &b.Task,
		"History":         &b.History,
		"Tool":            &b.Tools,
		"Flags":           &b.Flags,
		"Documents":       &b.Documents,
		"Meta":            &b.Meta,
		"Context":         &b.Context,
		"SystemDocuments": &b.SystemDocuments,
		"Elicitation":     &b.Elicitation,
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

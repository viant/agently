package prompt

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/pkg/mcpname"
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
	}

	Message struct {
		Kind       MessageKind   `yaml:"kind,omitempty" json:"kind,omitempty"`
		Role       string        `yaml:"role,omitempty" json:"role,omitempty"`
		MimeType   string        `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`
		Content    string        `yaml:"content,omitempty" json:"content,omitempty"`
		Attachment []*Attachment `yaml:"attachment,omitempty" json:"attachment,omitempty"`
		CreatedAt  time.Time     `yaml:"createdAt,omitempty" json:"createdAt,omitempty"`

		ID string `yaml:"id,omitempty" json:"id,omitempty"`

		// Optional tool metadata for tool result messages.
		ToolOpID    string                 `yaml:"toolOpId,omitempty" json:"toolOpId,omitempty"`
		ToolName    string                 `yaml:"toolName,omitempty" json:"toolName,omitempty"`
		ToolArgs    map[string]interface{} `yaml:"toolArgs,omitempty" json:"toolArgs,omitempty"`
		ToolTraceID string                 `yaml:"toolTraceId,omitempty" json:"toolTraceId,omitempty"`
	}

	Attachment struct {
		Name          string `yaml:"name,omitempty" json:"name,omitempty"`
		URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`
		StagingFolder string `yaml:"stagingFolder,omitempty" json:"stagingFolder,omitempty"`
		Mime          string `yaml:"mime,omitempty" json:"mime,omitempty"`
		Content       string `yaml:"content,omitempty" json:"content,omitempty"`
		Data          []byte `yaml:"data,omitempty" json:"data,omitempty"`
	}

	Turn struct {
		ID        string     `yaml:"id,omitempty" json:"id,omitempty"`
		StartedAt time.Time  `yaml:"startedAt,omitempty" json:"startedAt,omitempty"`
		Messages  []*Message `yaml:"messages,omitempty" json:"messages,omitempty"`
	}

	History struct {
		// Past contains committed conversation turns in chronological order.
		Past []*Turn `yaml:"past,omitempty" json:"past,omitempty"`

		// Current holds the in-flight turn for the ongoing request
		// (React loop step). It is intentionally separated from Past
		// so we can distinguish persisted history from the current
		// LLM call context.
		Current *Turn `yaml:"current,omitempty" json:"current,omitempty"`

		// CurrentTurnID, when non-empty, carries the id of the in-flight
		// turn associated with the Current messages. It is typically
		// aligned with memory.TurnMeta.TurnID and is provided for
		// observability and explicitness; Past remains the committed
		// transcript.
		CurrentTurnID string `yaml:"currentTurnID,omitempty" json:"currentTurnID,omitempty"`

		// Messages is a legacy flat view of persisted history kept for
		// template compatibility and internal tooling. New code should
		// prefer Past/Current and helper methods.
		Messages []*Message `yaml:"messages,omitempty" json:"messages,omitempty"`

		LastResponse *Trace            `yaml:"lastResponse,omitempty" json:"lastResponse,omitempty"`
		Traces       map[string]*Trace `yaml:"traces,omitempty" json:"traces,omitempty"`
		ToolExposure string
	}
)

type Kind string

const (
	KindResponse Kind = "resp"
	KindToolCall Kind = "op"
	KindContent  Kind = "content"
)

// MessageKind classifies high-level message semantics
// in history for LLM binding purposes.
type MessageKind string

const (
	MessageKindChatUser      MessageKind = "chat_user"
	MessageKindChatAssistant MessageKind = "chat_assistant"
	MessageKindToolResult    MessageKind = "tool_result"
	MessageKindElicitPrompt  MessageKind = "elicit_prompt"
	MessageKindElicitAnswer  MessageKind = "elicit_answer"
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

// ToLLM converts a prompt.Message into an llm.Message, preserving
// attachments and role. It sorts attachments by URI to ensure
// deterministic ordering when multiple attachments are present.
func (m *Message) ToLLM() llm.Message {
	if m == nil {
		return llm.Message{}
	}
	role := llm.MessageRole(m.Role)
	// Normalize any history messages with tool role to assistant
	// when converting to LLM messages. Structured tool results for
	// providers are created via llm.NewToolResultMessage and do not
	// pass through this helper.
	if strings.EqualFold(strings.TrimSpace(m.Role), "tool") {
		role = llm.RoleAssistant
	}
	if len(m.Attachment) == 0 {
		return llm.NewTextMessage(role, m.Content)
	}
	// Sort attachments by URI for stable order.
	sort.Slice(m.Attachment, func(i, j int) bool {
		if m.Attachment[i] == nil || m.Attachment[j] == nil {
			return false
		}
		return strings.Compare(m.Attachment[i].URI, m.Attachment[j].URI) < 0
	})
	items := make([]*llm.AttachmentItem, 0, len(m.Attachment))
	for _, a := range m.Attachment {
		if a == nil {
			continue
		}
		items = append(items, &llm.AttachmentItem{
			Name:     a.Name,
			MimeType: a.MIMEType(),
			Data:     a.Data,
			Content:  a.Content,
		})
	}
	return llm.NewMessageWithBinaries(role, items, m.Content)
}

// Messages flattens Past turns (and optionally Current when present)
// into a chronological slice of messages. It is intended for
// legacy/template usage and internal tooling; new LLM flows should
// prefer LLMMessages.
// Messages flattens Past into a legacy slice for compatibility.
// It intentionally excludes Current so overflow and trimming
// operate only on persisted history.
// NOTE: This is maintained as a field; callers should treat it
// as read-only and prefer Past/Current when possible.

// LLMMessages flattens history turns into a chronological slice of
// llm.Messages. When Past/Current are present, they are used as the
// source; otherwise the legacy flat Messages field is used.
func (h *History) LLMMessages() []llm.Message {
	var out []llm.Message
	if h == nil {
		return out
	}
	trimmedCurrentID := strings.TrimSpace(h.CurrentTurnID)
	appendLLM := func(msg *Message, omitTools bool) {
		if msg == nil {
			return
		}
		switch msg.Kind {
		case MessageKindToolResult:
			if omitTools {
				return
			}
			toolMsgs := toolResultLLMMessages(msg)
			out = append(out, toolMsgs...)
		case MessageKindElicitPrompt, MessageKindElicitAnswer:
			return
		default:
			out = append(out, msg.ToLLM())
		}
	}

	omitTools := h.ToolExposure == "turn"

	// Preferred path: flatten Past (committed transcript) and then
	// Current (in-flight turn) when present. Legacy clients that rely
	// solely on Messages will not populate Past/Current.
	if len(h.Past) > 0 || h.Current != nil {
		for _, t := range h.Past {
			if t == nil {
				continue
			}
			isCurrentTurn := trimmedCurrentID != "" && strings.TrimSpace(t.ID) == trimmedCurrentID
			omitToolsForTurn := omitTools && !isCurrentTurn
			for _, m := range t.Messages {
				appendLLM(m, omitToolsForTurn) // omit tool results if turn-level tool exposure and not current turn
			}
		}
		if h.Current != nil {
			for _, m := range h.Current.Messages {
				appendLLM(m, false)
			}
		}
		return out
	}
	// Fallback: legacy flat Messages view.
	for _, m := range h.Messages {
		appendLLM(m, false)
	}
	return out
}

func toolResultLLMMessages(msg *Message) []llm.Message {
	if msg == nil {
		return nil
	}
	opID := strings.TrimSpace(msg.ToolOpID)
	if opID == "" {
		return nil
	}
	rawName := strings.TrimSpace(msg.ToolName)
	name := mcpname.Canonical(rawName)
	if name == "" {
		name = rawName
	}
	result := strings.TrimSpace(msg.Content)
	call := llm.NewToolCall(opID, name, msg.ToolArgs, result)
	assistant := llm.NewAssistantMessageWithToolCalls(call)
	tool := newToolResultMessageWithAttachments(call, msg.Attachment)
	return []llm.Message{assistant, tool}
}

func newToolResultMessageWithAttachments(call llm.ToolCall, attachments []*Attachment) llm.Message {
	if len(attachments) == 0 {
		return llm.NewToolResultMessage(call)
	}
	items := make([]*llm.AttachmentItem, 0, len(attachments))
	for _, a := range attachments {
		if a == nil || len(a.Data) == 0 {
			continue
		}
		items = append(items, &llm.AttachmentItem{
			Name:     a.Name,
			MimeType: a.MIMEType(),
			Data:     a.Data,
			Content:  a.Content,
		})
	}
	if len(items) == 0 {
		return llm.NewToolResultMessage(call)
	}
	tool := llm.NewMessageWithBinaries(llm.RoleTool, items, call.Result)
	tool.Name = call.Name
	tool.ToolCallId = call.ID
	return tool
}

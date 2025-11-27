package llm

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ContentType defines the supported asset types.
type ContentType string

const (
	ContentTypeText   ContentType = "text"
	ContentTypeImage  ContentType = "image"
	ContentTypeVideo  ContentType = "video"
	ContentTypePDF    ContentType = "pdf"
	ContentTypeAudio  ContentType = "audio"
	ContentTypeBinary ContentType = "binary"

	// Legacy content types for backward compatibility
	ContentTypeImageURL ContentType = "image_url"
)

// AssetSource defines the way the asset is provided.
type AssetSource string

const (
	SourceURL    AssetSource = "url"
	SourceBase64 AssetSource = "base64"
	SourceRaw    AssetSource = "raw"
)

// ContentItem is a universal representation of any content asset in the message.
type ContentItem struct {
	Name string `json:"name,omitempty"`

	// Type indicates the type of the content.
	Type ContentType `json:"type"`

	// Source indicates how the asset is provided (url, base64, raw bytes).
	Source AssetSource `json:"source"`

	// Data is the actual content of the asset.
	// - For SourceURL: URL as string.
	// - For SourceBase64: Base64-encoded data.
	// - For SourceRaw: Raw binary data (usually base64 encoded or omitted in JSON).
	Data string `json:"data,omitempty"`

	MimeType string `json:"mimeType,omitempty"`

	// Metadata is optional structured metadata (e.g., for video timestamps, image detail levels).
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Legacy fields for backward compatibility
	Text string `json:"text,omitempty"`
}

// ImageURL represents an image referenced by URL.
// Deprecated: Use ContentItem with ContentTypeImage and SourceURL instead.
type ImageURL struct {
	// URL is the URL of the image.
	URL string `json:"url"`

	// Detail specifies the detail level for image analysis.
	// Options: "auto", "low", "high"
	Detail string `json:"detail,omitempty"`
}

// MessageRole represents the role of the message sender.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleFunction  MessageRole = "function"
	RoleTool      MessageRole = "tool"
)

func (m MessageRole) String() string {
	return string(m)
}

// Message is a generic message suitable for multiple content items and types.
type Message struct {
	// Role of the sender (user, assistant, system, etc.)
	Role MessageRole `json:"role"`

	// Name is the optional sender/tool name.
	Name string `json:"name,omitempty"`

	// Items contains multiple, diverse content assets.
	Items []ContentItem `json:"items,omitempty"`

	// ToolCalls represents structured function/tool calls.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// Legacy fields for backward compatibility
	Content      string        `json:"content,omitempty"`
	ToolCallId   string        `json:"tool_call_id,omitempty"`
	ContentItems []ContentItem `json:"content_items,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
}

// FunctionCall represents a function call made by the assistant.
// Deprecated: Use ToolCall instead.
type FunctionCall struct {
	// Name is the name of the function to call.
	Name string `json:"name"`

	// Arguments is a JSON string containing the arguments to pass to the function.
	Arguments string `json:"arguments"`
}

// ToolCall is a structured representation of a function/tool invocation.
type ToolCall struct {
	// ID is a unique identifier for the tool call.
	ID string `json:"id,omitempty"`

	// Name is the name of the tool to call.
	Name string `json:"name"`

	// Arguments contains the arguments to pass to the tool.
	Arguments map[string]interface{} `json:"arguments"`

	// Legacy fields for backward compatibility
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function,omitempty"`

	Result string `json:"result,omitempty"`
	//Error tool call error
	Error string `json:"error,omitempty"`
}

type CallSpan struct {
	StartedAt time.Time `json:"startedat"`
	EndedAt   time.Time `json:"endedat"`
}

type AttachmentItem struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     []byte `json:"data,omitempty"`
	Content  string `json:"content,omitempty"`
}

func (c *CallSpan) SetEnd(t time.Time) {
	c.EndedAt = t
}

// GenerateRequest represents a request to a chat-based LLM.
// It is designed to be compatible with various LLM providers.
type GenerateRequest struct {
	// Messages is the list of messages in the conversation.
	Messages []Message `json:"messages"`

	// Options contains additional options for the request.
	Options *Options `json:"options,omitempty"`

	// PreviousResponseID optionally links this request to a prior provider
	// response when the backend supports incremental continuation (e.g.,
	// OpenAI Responses API).
	PreviousResponseID string `json:"previous_response_id,omitempty"`
}

// GenerateResponse represents a response from a chat-based LLM.
// It is designed to be compatible with various LLM providers.
type GenerateResponse struct {
	// Choices contains the generated responses.
	Choices []Choice `json:"choices"`

	// Usage contains token usage information.
	Usage *Usage `json:"usage,omitempty"`
	Model string `json:"model,omitempty"`

	// ResponseID is a provider response identifier when available.
	ResponseID string `json:"response_id,omitempty"`
}

// Choice represents a single response choice from a chat-based LLM.
type Choice struct {
	// Index is the index of the choice.
	Index int `json:"index"`

	// Message is the generated message.
	Message Message `json:"message"`

	// FinishReason is the reason why the generation stopped.
	FinishReason string `json:"finish_reason,omitempty"`
}

// Usage contains token usage information.
type Usage struct {
	// PromptTokens is the number of tokens used in the prompt.
	PromptTokens int `json:"prompt_tokens"`

	// CompletionTokens is the number of tokens used in the completion.
	CompletionTokens int `json:"completion_tokens"`

	// TotalTokens is the total number of tokens used.
	TotalTokens int `json:"total_tokens"`

	// ContextTokens is the list of token IDs used in the model context (Ollama-specific).
	ContextTokens []int `json:"context_tokens,omitempty"`

	CachedTokens int `json:"cached_tokens,omitempty"`

	ReasoningTokens int `json:"reasoning_tokens,omitempty"`

	AudioTokens int `json:"audio_tokens,omitempty"`

	// Provider-detailed usage (OpenAI-compatible)
	// Prompt-level cached tokens (from prompt_tokens_details.cached_tokens)
	PromptCachedTokens int `json:"prompt_cached_tokens,omitempty"`
	// Prompt-level audio tokens (from prompt_tokens_details.audio_tokens)
	PromptAudioTokens int `json:"prompt_audio_tokens,omitempty"`
	// Completion-level reasoning tokens (from completion_tokens_details.reasoning_tokens)
	CompletionReasoningTokens int `json:"completion_reasoning_tokens,omitempty"`
	// Completion-level audio tokens (from completion_tokens_details.audio_tokens)
	CompletionAudioTokens int `json:"completion_audio_tokens,omitempty"`
	// Speculative decoding accepted/rejected prediction tokens
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

type Messages []Message

func (m *Messages) Append(msg Message) {
	*m = append(*m, msg)
}

// NewUserMessage creates a new message with the "user" role.
// NewUserMessage creates a new message with the "user" role.
func NewUserMessage(content string) Message {
	return NewTextMessage(RoleUser, content)
}

// NewSystemMessage creates a new message with the "system" role.
// NewSystemMessage creates a new message with the "system" role.
func NewSystemMessage(content string) Message {
	return NewTextMessage(RoleSystem, content)
}

// NewAssistantMessage creates a new message with the "assistant" role.
// NewAssistantMessage creates a new message with the "assistant" role.
func NewAssistantMessage(content string) Message {
	return NewTextMessage(RoleAssistant, content)
}

// NewToolMessage creates a new message with the "tool" role.
// NewToolMessage creates a new message with the "tool" role.
func NewToolMessage(name, content string) Message {
	msg := NewTextMessage(RoleTool, content)
	msg.Name = name
	return msg
}

// NewToolResultMessage creates a tool role message with the given tool call's ID and result content.
func NewToolResultMessage(call ToolCall) Message {
	content := call.Result
	if content == "" && call.Error != "" {
		content = "Error:" + call.Error
	}
	msg := NewTextMessage(RoleTool, content)
	msg.Name = call.Name
	msg.ToolCallId = call.ID
	return msg
}

// NewUserMessageWithImage creates a new message with the "user" role that includes both text and an image.
// NewUserMessageWithImage creates a new message with the "user" role that includes both text and an image.
func NewUserMessageWithImage(text, imageURL string, detail string) Message {
	textItem := NewTextContent(text)
	imageItem := ContentItem{
		Type:   ContentTypeImage,
		Source: SourceURL,
		Data:   imageURL,
		Metadata: map[string]interface{}{
			"detail": detail,
		},
	}
	return Message{
		Role:  RoleUser,
		Items: []ContentItem{textItem, imageItem},
	}
}

// NewContentItem creates a new content item with the specified type.
func NewContentItem(contentType ContentType) ContentItem {
	return ContentItem{
		Type: contentType,
	}
}

// NewTextContent creates a new text content item.
func NewTextContent(text string) ContentItem {
	return ContentItem{
		Type:   ContentTypeText,
		Source: SourceRaw,
		Data:   text,
		Text:   text, // For backward compatibility
	}
}

// NewImageContent creates a new image content item.
func NewImageContent(imageURL string, detail string) ContentItem {
	// Create a new image content item using the preferred approach
	return ContentItem{
		// Preferred approach: Use Type=ContentTypeImage, Source=SourceURL, and Data field
		Type:   ContentTypeImage,
		Source: SourceURL,
		Data:   imageURL,
		Metadata: map[string]interface{}{
			"detail": detail,
		},
	}
}

// NewBinaryContent creates a new binary content item from raw data.
func NewBinaryContent(data []byte, mimeType, name string) ContentItem {
	encoded := base64.StdEncoding.EncodeToString(data)
	return ContentItem{
		Name:     name,
		Type:     ContentTypeBinary,
		Source:   SourceBase64,
		Data:     encoded,
		MimeType: mimeType,
	}
}

// NewUserMessageWithBinary creates a new user message that includes binary data and optional text prompt.
func NewUserMessageWithBinary(data []byte, mimeType, prompt, name string) Message {
	items := []ContentItem{NewBinaryContent(data, mimeType, name)}
	if prompt != "" {
		items = append(items, NewTextContent(prompt))
	}
	return Message{Role: RoleUser, Items: items}
}

// NewMessageWithBinary creates a message that includes binary data and optional text prompt.
func NewMessageWithBinary(role MessageRole, data []byte, mimeType, content, name string) Message {
	items := []ContentItem{NewBinaryContent(data, mimeType, name)}
	if content != "" {
		items = append(items, NewTextContent(content))
	}
	return Message{Role: role, Items: items}
}

func NewMessageWithBinaries(role MessageRole, attachItems []*AttachmentItem, content string) Message {
	items := []ContentItem{}

	for _, a := range attachItems {
		items = append(items, NewBinaryContent(a.Data, a.MimeType, a.Name))
	}

	if content != "" {
		items = append(items, NewTextContent(content))
	}

	return Message{Role: role, Items: items}
}

// NewTextMessage creates a text-only message for the given role.
func NewTextMessage(role MessageRole, content string) Message {
	textItem := NewTextContent(content)
	return Message{
		Role:    role,
		Items:   []ContentItem{textItem},
		Content: content, // For backward compatibility
	}
}

// NewFunctionCall creates a FunctionCall with the given name and arguments.
func NewFunctionCall(name string, args map[string]interface{}) FunctionCall {
	data, _ := json.Marshal(args)
	return FunctionCall{
		Name:      name,
		Arguments: string(data),
	}
}

// NewToolCall creates a ToolCall with the given function name and arguments.
// An ID is generated automatically and legacy fields are populated for backward compatibility.
func NewToolCall(id string, name string, args map[string]interface{}, result string) ToolCall {
	if id == "" {
		id = uuid.NewString()
	}
	// Copy args to avoid modification of the input map
	copied := make(map[string]interface{}, len(args))
	for key, val := range args {
		copied[key] = val
	}
	fc := NewFunctionCall(name, copied)
	return ToolCall{
		ID:        id,
		Name:      name,
		Arguments: copied,
		Type:      "function",
		Function:  fc,
		Result:    result,
	}
}

// NewAssistantMessageWithToolCalls creates an assistant message that includes tool calls.
func NewAssistantMessageWithToolCalls(toolCalls ...ToolCall) Message {
	return Message{
		Role:      RoleAssistant,
		ToolCalls: toolCalls,
	}
}

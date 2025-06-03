package claude

// Request represents the request structure for Claude API on Vertex AI
type Request struct {
	AnthropicVersion string    `json:"anthropic_version"`
	Messages         []Message `json:"messages"`
	MaxTokens        int       `json:"max_tokens,omitempty"`
	Stream           bool      `json:"stream,omitempty"`
	Thinking         *Thinking `json:"thinking,omitempty"`
	System           string    `json:"system,omitempty"`
}

// Message represents a message in the Claude API request
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in a message
type ContentBlock struct {
	Type   string  `json:"type"`
	Text   string  `json:"text,omitempty"`
	Source *Source `json:"source,omitempty"`
	// Tool request coming FROM the model
	ToolUse *ToolUseBlock `json:"toolUse,omitempty"`

	// Result you send back TO the model
	ToolResult *ToolResultBlock `json:"toolResult,omitempty"`
}

type ToolUseBlock struct {
	ToolUseId string                 `json:"toolUseId"` // <— the correlation handle
	Name      string                 `json:"name"`      // must match a ToolDefinition.Name
	Input     map[string]interface{} `json:"input"`     // validated by InputSchema
}

type ToolResultBlock struct {
	ToolUseId string                   `json:"toolUseId"`         // echo back unchanged
	Content   []ToolResultContentBlock `json:"content,omitempty"` // result payload
	Status    string                   `json:"status,omitempty"`  // "success" | "error" (Claude-only)
}

type ToolResultContentBlock struct {
	Text *string     `json:"text,omitempty"`
	JSON interface{} `json:"json,omitempty"` // any JSON-serialisable value
}

// Source represents a source for image content
type Source struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
}

// Thinking represents the thinking configuration for Claude
type Thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// Response represents the response structure from Claude API
type Response struct {
	Type    string  `json:"type"`
	Message Message `json:"message,omitempty"`
	Delta   *Delta  `json:"delta,omitempty"`
	Error   *Error  `json:"error,omitempty"`

	// VertexAI specific fields
	ID           string        `json:"id,omitempty"`
	Role         string        `json:"role,omitempty"`
	Model        string        `json:"model,omitempty"`
	Content      []interface{} `json:"content,omitempty"`
	StopReason   string        `json:"stop_reason,omitempty"`
	StopSequence string        `json:"stop_sequence,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}

// Delta represents a delta in the streaming response
type Delta struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

// Error represents an error in the response
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// VertexAIResponse represents the response structure from VertexAI Claude API
type VertexAIResponse struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Role         string        `json:"role"`
	Model        string        `json:"model"`
	Content      []TextContent `json:"content"`
	StopReason   string        `json:"stop_reason"`
	StopSequence string        `json:"stop_sequence"`
	Usage        *Usage        `json:"usage"`
}

// TextContent represents a text content block in the VertexAI response
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

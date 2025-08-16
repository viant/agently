package claude

// Request represents the request structure for Claude API on Vertex AI
// Request represents the request structure for Claude API on Vertex AI
// The schema aligns with Anthropic's tool-calling contract that is also
// used by AWS Bedrock so the same high-level llm.GenerateRequest can be
// marshalled to either provider.
type Request struct {
	AnthropicVersion string           `json:"anthropic_version"`
	Messages         []Message        `json:"messages"`
	Tools            []ToolDefinition `json:"tools,omitempty"`

	// optional generation parameters --------------------------------------------------
	MaxTokens     int      `json:"max_tokens,omitempty"`
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Vertex AI specific extensions ---------------------------------------------------
	Stream   bool      `json:"stream,omitempty"`
	Thinking *Thinking `json:"thinking,omitempty"`
	System   string    `json:"system,omitempty"`
}

// Message represents a message in the Claude API request
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in a message
// ContentBlock represents a content block in a message.
// For consistency with AWS Bedrock implementation we keep a flat structure
// where the "type" field distinguishes: "text", "image", "tool_use",
// "tool_result", ...
type ContentBlock struct {
	// Common -------------------------------------------------------------------------
	Type string `json:"type"`

	// Text content (when Type == "text") -------------------------------------------
	Text   string  `json:"text,omitempty"`
	Source *Source `json:"source,omitempty"`

	// Tool invocation (when Type == "tool_use") -------------------------------------
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`

	// Tool result (when Type == "tool_result") --------------------------------------
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   interface{} `json:"content,omitempty"`
}

// ToolDefinition mirrors the JSON schema Claude expects when declaring tools.
type ToolDefinition struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	OutputSchema map[string]interface{} `json:"output_schema,omitempty"`
}

// The previous granular structs (ToolUseBlock, ToolResultBlock, ... ) have been
// unified into the flat representation above because both Vertex AI and Bedrock
// accept that shape and it keeps the adapter code provider-agnostic.

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

	// Streaming specific helpers
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
}

// Delta represents a delta in the streaming response
type Delta struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
	// For tool input deltas (type == "input_json_delta")
	PartialJSON string `json:"partial_json,omitempty"`
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
	Type  string                 `json:"type"`
	Text  string                 `json:"text"`
	Id    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

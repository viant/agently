package claude

// Request represents the request structure for Claude API on AWS Bedrock
type Request struct {
	AnthropicVersion string      `json:"anthropic_version"`
	Messages         []Message   `json:"messages"`
	ToolConfig       *ToolConfig `json:"toolConfig,omitempty"` // tool definitions

	MaxTokens     int      `json:"max_tokens,omitempty"`
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
	System        string   `json:"system,omitempty"`
}

// ToolConfig wraps the list of tools you expose to the model.
type ToolConfig struct {
	Tools []ToolDefinition `json:"tools"`
}

// ToolDefinition mirrors the JSON schema Bedrock expects.
type ToolDefinition struct {
	Name         string                 `json:"name"`                   // snake_case
	Description  string                 `json:"description,omitempty"`  // shown to the model
	InputSchema  map[string]interface{} `json:"inputSchema"`            // JSON Schema Draft-07
	OutputSchema map[string]interface{} `json:"outputSchema,omitempty"` // JSON Schema Draft-07
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

// Response represents the response structure from Claude API on AWS Bedrock
type Response struct {
	ID                             string             `json:"id"`
	Type                           string             `json:"type"`
	Role                           string             `json:"role"`
	Content                        []ContentItem      `json:"content"`
	Model                          string             `json:"model"`
	StopReason                     string             `json:"stop_reason"`
	StopSequence                   string             `json:"stop_sequence"`
	Usage                          *Usage             `json:"usage"`
	AmazonBedrockInvocationMetrics *InvocationMetrics `json:"amazon-bedrock-invocationMetrics,omitempty"`
}

// ContentItem represents a content item in the response
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// InvocationMetrics represents AWS Bedrock invocation metrics
type InvocationMetrics struct {
	InputTokenCount   int `json:"inputTokenCount"`
	OutputTokenCount  int `json:"outputTokenCount"`
	InvocationLatency int `json:"invocationLatency"`
	FirstByteLatency  int `json:"firstByteLatency"`
}

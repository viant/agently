package claude

// Request represents the request structure for Claude API on AWS Bedrock
type Request struct {
	AnthropicVersion string    `json:"anthropic_version"`
	Messages         []Message `json:"messages"`
	MaxTokens        int       `json:"max_tokens,omitempty"`
	Temperature      float64   `json:"temperature,omitempty"`
	TopP             float64   `json:"top_p,omitempty"`
	TopK             int       `json:"top_k,omitempty"`
	StopSequences    []string  `json:"stop_sequences,omitempty"`
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

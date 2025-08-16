package openai

import (
	"github.com/viant/agently/genai/llm"
)

// Request represents the request structure for OpenAI API
type Request struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Temperature   *float64       `json:"temperature,omitempty"`
	MaxTokens     int            `json:"max_completion_tokens,omitempty"`
	TopP          float64        `json:"top_p,omitempty"`
	N             int            `json:"n,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	// Reasoning enables configuration of internal chain-of-thought reasoning features.
	Reasoning  *llm.Reasoning `json:"reasoning,omitempty"`
	Tools      []Tool         `json:"tools,omitempty"`
	ToolChoice interface{}    `json:"tool_choice,omitempty"`
}

// StreamOptions controls additional streaming behavior.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ContentItem represents a single content item in a message for the OpenAI API
type ContentItem struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image referenced by URL for the OpenAI API
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Message represents a message in the OpenAI API request
type Message struct {
	Role         string        `json:"role"`
	Content      interface{}   `json:"content,omitempty"` // Can be string or []ContentItem
	Name         string        `json:"name,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallId   string        `json:"tool_call_id,omitempty"`
}

// FunctionCall represents a function call in the OpenAI API
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall represents a tool call in the OpenAI API
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// Tool represents a tool in the OpenAI API
type Tool struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

// ToolDefinition represents a tool definition in the OpenAI API
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Required    []string               `json:"required,omitempty"`
}

// Response represents the response structure from OpenAI API
type Response struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a choice in the OpenAI API response
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage information in the OpenAI API response
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Streaming chunk types (SSE) -------------------------------------------------

// StreamResponse represents a single Server-Sent Event chunk from OpenAI
// chat/completions endpoint when stream=true. The payload places partial deltas
// under choices[i].delta instead of choices[i].message.
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        DeltaMessage `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

type DeltaMessage struct {
	Role      string          `json:"role,omitempty"`
	Content   *string         `json:"content,omitempty"`
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
}

// ToolCallDelta mirrors the incremental tool call fields included in streaming
// deltas. Arguments are delivered as a concatenated string across multiple
// events.
type ToolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function FunctionCallDelta `json:"function,omitempty"`
}

type FunctionCallDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

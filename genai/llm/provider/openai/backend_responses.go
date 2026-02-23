package openai

import "strings"

// ChatGPTBackendResponsesPayload is the dedicated request contract for
// https://chatgpt.com/backend-api/codex/responses.
//
// Do not add v1/responses-only fields here (e.g. temperature, top_p, n,
// max_output_tokens). Backend rejects several of those.
type ChatGPTBackendResponsesPayload struct {
	Model             string          `json:"model"`
	Instructions      string          `json:"instructions"`
	Input             []InputItem     `json:"input"`
	Tools             []ResponsesTool `json:"tools,omitempty"`
	ToolChoice        interface{}     `json:"tool_choice,omitempty"`
	ParallelToolCalls bool            `json:"parallel_tool_calls,omitempty"`
	Reasoning         interface{}     `json:"reasoning,omitempty"`
	Store             bool            `json:"store"`
	Stream            bool            `json:"stream"`
	Include           []string        `json:"include,omitempty"`
	PromptCacheKey    string          `json:"prompt_cache_key,omitempty"`
	Text              *TextControls   `json:"text,omitempty"`
}

// ToChatGPTBackendResponsesPayload builds a backend-specific payload.
// It intentionally applies backend constraints and role adaptations that differ
// from OpenAI v1/responses.
func ToChatGPTBackendResponsesPayload(req *Request) *ChatGPTBackendResponsesPayload {
	base := ToResponsesPayload(req)
	adaptSystemMessagesForChatGPTBackend(base)

	instructions := strings.TrimSpace(base.Instructions)
	if instructions == "" {
		instructions = "You are a helpful assistant."
	}

	out := &ChatGPTBackendResponsesPayload{
		Model:             base.Model,
		Instructions:      instructions,
		Input:             base.Input,
		Tools:             base.Tools,
		ToolChoice:        base.ToolChoice,
		ParallelToolCalls: base.ParallelToolCalls,
		Reasoning:         base.Reasoning,
		Store:             false, // backend requires false
		Stream:            true,  // backend requires true
		Include:           base.Include,
		PromptCacheKey:    strings.TrimSpace(base.PromptCacheKey),
		Text:              base.Text,
	}
	return out
}

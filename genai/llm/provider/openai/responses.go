package openai

import (
	"encoding/json"
	"strings"

	"github.com/viant/agently/genai/llm"
)

// ResponsesPayload is the request body for the /v1/responses API.
type ResponsesPayload struct {
	Model              string          `json:"model"`
	Input              []interface{}   `json:"input"`
	Tools              []ResponsesTool `json:"tools,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	MaxOutputTokens    int             `json:"max_output_tokens,omitempty"`
	TopP               float64         `json:"top_p,omitempty"`
	N                  int             `json:"n,omitempty"`
	Stream             bool            `json:"stream,omitempty"`
	ToolChoice         interface{}     `json:"tool_choice,omitempty"`
	ParallelToolCalls  bool            `json:"parallel_tool_calls,omitempty"`
	Reasoning          *llm.Reasoning  `json:"reasoning,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Store              *bool           `json:"store,omitempty"`
	// Provider-specific metadata passthrough if needed in future
	Extra map[string]interface{} `json:"-"`
}

type ResponsesInput struct {
	Role string `json:"role"`
	// Name is not supported by Responses API and must be omitted.
	Content []ResponsesContentItem `json:"content"`
	// ToolCallID is required by OpenAI when role == "tool" to associate
	// the tool result with a prior assistant tool_call request.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type ResponsesContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	FileName string `json:"filename,omitempty"`
	Detail   string `json:"detail,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	// Function call output back to the model
	CallID string `json:"call_id,omitempty"`
	Output string `json:"output,omitempty"`
	// Allow passthrough for future shapes (e.g., input_audio)
	Extra map[string]interface{} `json:"-"`
}

// ResponsesTool is the Responses API tool schema. For function tools, the
// name/description/parameters are top-level (not nested under "function").
type ResponsesTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type ResponsesImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ResponsesResponse represents a non-streaming reply from the /v1/responses API
// or the final object emitted in the streaming completed event.
type ResponsesResponse struct {
	ID     string                `json:"id"`
	Status string                `json:"status,omitempty"`
	Model  string                `json:"model"`
	Output []ResponsesOutputItem `json:"output"`
	Usage  ResponsesUsage        `json:"usage"`
}

type ResponsesOutputItem struct {
	// Type is typically "message".
	Type    string                 `json:"type"`
	Role    string                 `json:"role,omitempty"`
	Content []ResponsesContentItem `json:"content,omitempty"`
	// Tool calls when assistant requests tools
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Function-call item shape (when Type == "function_call")
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ToResponsesPayload converts legacy Request into Responses API payload.
func ToResponsesPayload(req *Request) *ResponsesPayload {
	if req == nil {
		return &ResponsesPayload{}
	}
	out := &ResponsesPayload{
		Model:              req.Model,
		Temperature:        req.Temperature,
		MaxOutputTokens:    req.MaxTokens,
		TopP:               req.TopP,
		N:                  req.N,
		Stream:             req.Stream,
		ToolChoice:         req.ToolChoice,
		ParallelToolCalls:  req.ParallelToolCalls,
		Reasoning:          req.Reasoning,
		PreviousResponseID: req.PreviousResponseID,
	}

	// Normalize tool_choice for Responses API: {type:"function", name:"..."}
	if m, ok := req.ToolChoice.(map[string]interface{}); ok {
		if t, _ := m["type"].(string); strings.EqualFold(t, "function") {
			// legacy shape may be { type: function, function: { name } }
			if f, okf := m["function"].(map[string]interface{}); okf {
				if name, _ := f["name"].(string); strings.TrimSpace(name) != "" {
					out.ToolChoice = map[string]interface{}{"type": "function", "name": name}
				}
			}
		}
	}
	// Map tools to Responses API schema
	if len(req.Tools) > 0 {
		out.Tools = make([]ResponsesTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			rt := ResponsesTool{Type: t.Type}
			// For function tools, move fields to top-level
			if strings.EqualFold(t.Type, "function") {
				rt.Name = t.Function.Name
				rt.Description = t.Function.Description
				rt.Parameters = t.Function.Parameters
			}
			out.Tools = append(out.Tools, rt)
		}
	}

	// Convert Messages to Input content
	out.Input = make([]interface{}, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := strings.TrimSpace(m.Role)
		isTool := role == "tool"
		if isTool {
			// Responses API does not accept role "tool"; map to user.
			role = "user"
		}

		isAssistant := role == "assistant"
		var items []ResponsesContentItem

		// Special-case: tool result messages → append function_call_output item directly
		if strings.TrimSpace(m.ToolCallId) != "" || strings.ToLower(m.Role) == "tool" {
			var outTxt string
			switch content := m.Content.(type) {
			case string:
				outTxt = strings.TrimSpace(content)
			case []ContentItem:
				var sb strings.Builder
				for _, it := range content {
					if it.Text != "" {
						sb.WriteString(it.Text)
					}
				}
				outTxt = strings.TrimSpace(sb.String())
			}
			// Only emit function_call_output when continuing a stored response
			if strings.TrimSpace(req.PreviousResponseID) != "" && strings.TrimSpace(m.ToolCallId) != "" && outTxt != "" {
				outTxt = strings.TrimSpace(outTxt)
				if !(strings.HasPrefix(outTxt, "{") || strings.HasPrefix(outTxt, "[")) {
					var data = map[string]interface{}{
						"data": outTxt,
					}
					payload, _ := json.Marshal(data)
					outTxt = string(payload)
				}
				out.Input = append(out.Input, ResponsesContentItem{Type: "function_call_output", CallID: m.ToolCallId, Output: outTxt})
				continue
			}
			// Otherwise fall through to normal message mapping (as input_text),
			// since function_call_output requires a previous_response_id.
		} else {
			switch content := m.Content.(type) {
			case string:
				t := "input_text"
				if isAssistant {
					t = "output_text"
				}
				if strings.TrimSpace(content) != "" {
					items = append(items, ResponsesContentItem{Type: t, Text: strings.TrimSpace(content)})
				}
			case []ContentItem:
				for _, it := range content {
					switch strings.ToLower(it.Type) {
					case "text":
						t := "input_text"
						if isAssistant {
							t = "output_text"
						}
						items = append(items, ResponsesContentItem{Type: t, Text: coalesce(it.Text)})
					case "image_url":
						var detail string
						if it.ImageURL != nil {
							detail = it.ImageURL.Detail
						}
						url := ""
						if it.ImageURL != nil {
							url = it.ImageURL.URL
						}
						items = append(items, ResponsesContentItem{Type: "input_image", ImageURL: url, Detail: detail})
					case "file":
						if it.File != nil && it.File.FileID != "" {
							items = append(items, ResponsesContentItem{Type: "input_file", FileID: it.File.FileID})
						} else {
							items = append(items, ResponsesContentItem{Type: "input_file", FileName: it.File.FileName, FileData: it.File.FileData})
						}
					default:
						// Fallback attempt: treat any other type with text
						t := "input_text"
						if isAssistant {
							t = "output_text"
						}
						items = append(items, ResponsesContentItem{Type: t, Text: it.Text})
					}
				}
			case []interface{}:
				for _, raw := range content {
					if mp, ok := raw.(map[string]interface{}); ok {
						typ, _ := mp["type"].(string)
						switch strings.ToLower(typ) {
						case "text":
							t := "input_text"
							if isAssistant {
								t = "output_text"
							}
							if v, _ := mp["text"].(string); strings.TrimSpace(v) != "" {
								items = append(items, ResponsesContentItem{Type: t, Text: strings.TrimSpace(v)})
							}
						case "image_url":
							var url, detail string
							if iu, ok := mp["image_url"].(map[string]interface{}); ok {
								if u, _ := iu["url"].(string); u != "" {
									url = u
								}
								if d, _ := iu["detail"].(string); d != "" {
									detail = d
								}
							}
							if url != "" {
								items = append(items, ResponsesContentItem{Type: "input_image", ImageURL: url, Detail: detail})
							}
						}
					}
				}
			}
		}

		// Skip messages with no content. Responses API rejects null content.
		if len(items) == 0 {
			// For assistant with no content/tool-calls, skip entirely
			// (assistant tool requests are generated by the model, not input).
			if role == "assistant" {
				continue
			}
			// For any role with no content at all, skip to avoid null content.
			continue
		}

		// Responses API does not accept top-level name; omit it.
		in := ResponsesInput{Role: role, Content: items}
		out.Input = append(out.Input, in)
	}
	return out
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ToLLMSFromResponses converts a Responses API response to llm.GenerateResponse.
func ToLLMSFromResponses(resp *ResponsesResponse) *llm.GenerateResponse {
	if resp == nil {
		return &llm.GenerateResponse{}
	}
	out := &llm.GenerateResponse{Model: resp.Model, ResponseID: resp.ID}
	// Convert output items to choices in order
	for i, item := range resp.Output {
		itype := strings.ToLower(strings.TrimSpace(item.Type))
		switch itype {
		case "message":
			msg := llm.Message{}
			switch strings.ToLower(item.Role) {
			case "system":
				msg.Role = llm.RoleSystem
			case "user":
				msg.Role = llm.RoleUser
			case "assistant":
				msg.Role = llm.RoleAssistant
			case "tool":
				msg.Role = llm.RoleTool
			default:
				msg.Role = llm.RoleAssistant
			}
			var sb strings.Builder
			for _, c := range item.Content {
				if c.Text != "" {
					sb.WriteString(c.Text)
				}
			}
			msg.Content = sb.String()
			if len(item.ToolCalls) > 0 {
				msg.ToolCalls = make([]llm.ToolCall, 0, len(item.ToolCalls))
				for _, tc := range item.ToolCalls {
					var args map[string]interface{}
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
					msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args, Type: tc.Type, Function: llm.FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments}})
				}
			}
			out.Choices = append(out.Choices, llm.Choice{Index: i, Message: msg})
		case "function_call":
			// Map a function_call output item to an assistant message with tool call
			msg := llm.Message{Role: llm.RoleAssistant}
			var args map[string]interface{}
			_ = json.Unmarshal([]byte(item.Arguments), &args)
			msg.ToolCalls = []llm.ToolCall{{
				ID:        item.CallID,
				Name:      item.Name,
				Arguments: args,
				Type:      "function",
				Function:  llm.FunctionCall{Name: item.Name, Arguments: item.Arguments},
			}}
			out.Choices = append(out.Choices, llm.Choice{Index: i, Message: msg})
		}
	}
	// Map usage
	out.Usage = &llm.Usage{PromptTokens: resp.Usage.InputTokens, CompletionTokens: resp.Usage.OutputTokens, TotalTokens: resp.Usage.TotalTokens}
	return out
}

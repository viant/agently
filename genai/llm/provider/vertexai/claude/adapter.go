package claude

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/viant/afs"
	"github.com/viant/agently/genai/llm"
	"strings"
)

// ToRequest converts a generic llm.GenerateRequest into a provider specific
// Claude request expected by Vertex AI. The logic mirrors the Bedrock adapter
// so that a single llm.GenerateRequest containing tools, images, etc. can be
// sent to either provider.
func ToRequest(ctx context.Context, request *llm.GenerateRequest) (*Request, error) {
	if request == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	req := &Request{
		AnthropicVersion: defaultAnthropicVersion,
	}

	// copy generation options --------------------------------------------------------
	if request.Options != nil {
		if request.Options.MaxTokens > 0 {
			req.MaxTokens = request.Options.MaxTokens
		}
		if request.Options.Temperature > 0 {
			req.Temperature = request.Options.Temperature
		}
		if request.Options.TopP > 0 {
			req.TopP = request.Options.TopP
		}
		if request.Options.TopK > 0 {
			req.TopK = request.Options.TopK
		}
		if len(request.Options.StopWords) > 0 {
			req.StopSequences = request.Options.StopWords
		}

		req.Stream = request.Options.Stream

		if thinking := request.Options.Thinking; thinking != nil {
			req.Thinking = &Thinking{
				Type:         thinking.Type,
				BudgetTokens: thinking.BudgetTokens,
			}
		}
	}

	// tools -------------------------------------------------------------------------
	if request.Options != nil && len(request.Options.Tools) > 0 {
		for _, tool := range request.Options.Tools {
			var inputSchema map[string]interface{}
			if tool.Definition.Parameters != nil {
				if _, hasType := tool.Definition.Parameters["type"]; hasType {
					inputSchema = tool.Definition.Parameters
				} else {
					inputSchema = map[string]interface{}{
						"type":       "object",
						"properties": tool.Definition.Parameters,
					}
				}
			} else {
				inputSchema = map[string]interface{}{"type": "object"}
			}
			if len(tool.Definition.Required) > 0 {
				if _, exists := inputSchema["required"]; !exists {
					inputSchema["required"] = tool.Definition.Required
				}
			}

			req.Tools = append(req.Tools, ToolDefinition{
				Name:        tool.Definition.Name,
				Description: tool.Definition.Description,
				InputSchema: inputSchema,
				// OutputSchema intentionally omitted – Claude currently ignores it.
			})
		}
	}

	// system message ----------------------------------------------------------------
	if strings.TrimSpace(request.Instructions) != "" {
		req.System = strings.TrimSpace(request.Instructions)
	}
	for _, msg := range request.Messages {
		if msg.Role == llm.RoleSystem {
			req.System = llm.MessageText(msg)
			break
		}
	}

	// messages ----------------------------------------------------------------------
	fs := afs.New()
	for _, msg := range request.Messages {
		// skip the system as it's handled via req.System
		if msg.Role == llm.RoleSystem {
			continue
		}

		// tool invocations requested by assistant -----------------------------------
		if len(msg.ToolCalls) > 0 {
			var blocks []ContentBlock
			for _, tc := range msg.ToolCalls {
				id := tc.ID
				if id == "" {
					id = tc.Name
				}
				blocks = append(blocks, ContentBlock{
					Type:  "tool_use",
					ID:    id,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			req.Messages = append(req.Messages, Message{
				Role:    string(msg.Role),
				Content: blocks,
			})
			continue
		}

		// tool result returned by caller --------------------------------------------
		if msg.Role == llm.RoleTool && msg.ToolCallId != "" {
			var resultContent interface{}
			if len(msg.Items) > 0 {
				resultContent = msg.Items[0].Data
			} else if msg.Content != "" {
				resultContent = msg.Content
			}
			req.Messages = append(req.Messages, Message{
				Role: "user", // as per Anthropic spec
				Content: []ContentBlock{{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallId,
					Content:   resultContent,
				}},
			})
			continue
		}

		// standard user/assistant messages -----------------------------------------
		claudeMsg := Message{Role: string(msg.Role)}

		for _, item := range msg.Items {
			switch item.Type {
			case llm.ContentTypeText:
				text := item.Text
				if msg.Role == llm.RoleUser && strings.TrimSpace(msg.Name) != "" {
					text = msg.Name + ":" + text
				}
				claudeMsg.Content = append(claudeMsg.Content, ContentBlock{
					Type: "text",
					Text: text,
				})
			case llm.ContentTypeImage:
				cblock, err := handleImageContent(ctx, fs, item)
				if err != nil {
					return nil, err
				}
				claudeMsg.Content = append(claudeMsg.Content, *cblock)
			default:
				return nil, fmt.Errorf("unsupported content type: %s", item.Type)
			}
		}

		if len(claudeMsg.Content) == 0 && msg.Content != "" {
			text := msg.Content
			if msg.Role == llm.RoleUser && strings.TrimSpace(msg.Name) != "" {
				text = msg.Name + ":" + text
			}
			claudeMsg.Content = append(claudeMsg.Content, ContentBlock{
				Type: "text",
				Text: text,
			})
		}

		req.Messages = append(req.Messages, claudeMsg)
	}

	return req, nil
}

// handleImageContent converts llm.ContentItem with an image into a Claude content
// block, encoding raw data to base64 if necessary.
func handleImageContent(ctx context.Context, fs afs.Service, item llm.ContentItem) (*ContentBlock, error) {
	var imageData string
	var mediaType string

	switch item.Source {
	case llm.SourceBase64:
		imageData = item.Data
		mediaType = item.MimeType
	case llm.SourceRaw:
		mediaType = item.MimeType
		if mediaType == "" {
			mediaType = "image/png"
		}
		imageData = base64.StdEncoding.EncodeToString([]byte(item.Data))
	case llm.SourceURL:
		return nil, fmt.Errorf("URL source not supported for Claude API, use base64 encoding instead")
	default:
		return nil, fmt.Errorf("unsupported image source: %s", item.Source)
	}

	return &ContentBlock{
		Type: "image",
		Source: &Source{
			Type:      "base64",
			MediaType: mediaType,
			Data:      imageData,
		},
	}, nil
}

// ToLLMSResponse converts a Claude API response into the generic llm.GenerateResponse.
// It supports both Vertex AI Claude response format and the standard Anthropic
// format (same as Bedrock).
func ToLLMSResponse(resp *Response) *llm.GenerateResponse {
	if resp == nil {
		return &llm.GenerateResponse{Choices: []llm.Choice{}}
	}

	// Detect Vertex AI style (top-level Content []interface{}) ----------------------
	if resp.ID != "" && resp.Content != nil {
		return handleVertexAIResponse(resp)
	}

	// Standard Anthropic/Bedrock style ---------------------------------------------
	var (
		fullText  string
		items     []llm.ContentItem
		toolCalls []llm.ToolCall
	)

	for _, content := range resp.Message.Content {
		switch content.Type {
		case "text":
			fullText += content.Text
			items = append(items, llm.ContentItem{
				Type:   llm.ContentTypeText,
				Source: llm.SourceRaw,
				Data:   content.Text,
				Text:   content.Text,
			})
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        content.ID,
				Name:      content.Name,
				Arguments: content.Input,
			})
		case "image":
			// ignore images in assistant response for now
		}
	}

	msg := llm.Message{
		Role:      llm.RoleAssistant,
		Content:   fullText,
		Items:     items,
		ToolCalls: toolCalls,
	}

	usage := &llm.Usage{}
	if resp.Usage != nil {
		usage.PromptTokens = resp.Usage.InputTokens
		usage.CompletionTokens = resp.Usage.OutputTokens
		usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens
	}

	return &llm.GenerateResponse{
		Choices: []llm.Choice{{
			Index:        0,
			Message:      msg,
			FinishReason: resp.StopReason,
		}},
		Usage: usage,
		Model: resp.Model,
	}
}

// handleVertexAIResponse retains the previous specialised logic but now also
// creates tool calls if present in the response content.
func handleVertexAIResponse(resp *Response) *llm.GenerateResponse {
	var (
		fullText  string
		items     []llm.ContentItem
		toolCalls []llm.ToolCall
	)

	for _, raw := range resp.Content {
		block, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch block["type"].(string) {
		case "text":
			text, _ := block["text"].(string)
			fullText += text
			items = append(items, llm.ContentItem{
				Type:   llm.ContentTypeText,
				Source: llm.SourceRaw,
				Data:   text,
				Text:   text,
			})
		case "tool_use":
			// Extract structured tool invocation details
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			arguments, _ := block["input"].(map[string]interface{})
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        id,
				Name:      name,
				Arguments: arguments,
			})
		}
	}

	var usage *llm.Usage
	if resp.Usage != nil {
		usage = &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return &llm.GenerateResponse{
		Choices: []llm.Choice{{
			Index: 0,
			Message: llm.Message{
				Role:      llm.MessageRole(resp.Role),
				Content:   fullText,
				Items:     items,
				ToolCalls: toolCalls,
			},
			FinishReason: resp.StopReason,
		}},
		Usage: usage,
		Model: resp.Model,
	}
}

// VertexAIResponseToLLMS converts the older VertexAI Claude response struct to
// the generic llm.GenerateResponse.  It is preserved to keep API.go compile
// happy and to satisfy existing tests.
func VertexAIResponseToLLMS(resp *VertexAIResponse) *llm.GenerateResponse {
	if resp == nil {
		return &llm.GenerateResponse{Choices: []llm.Choice{}}
	}

	var (
		fullText  string
		items     []llm.ContentItem
		toolCalls []llm.ToolCall
	)
	for _, part := range resp.Content {
		if part.Type == "text" {
			fullText += part.Text
			items = append(items, llm.ContentItem{
				Type:   llm.ContentTypeText,
				Source: llm.SourceRaw,
				Data:   part.Text,
				Text:   part.Text,
			})
		}
		if part.Type == "tool_use" {
			toolCall := llm.ToolCall{
				ID:        part.Id,
				Name:      part.Name,
				Arguments: part.Input,
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}

	var usage *llm.Usage
	if resp.Usage != nil {
		usage = &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return &llm.GenerateResponse{
		Choices: []llm.Choice{{
			Index: 0,
			Message: llm.Message{
				Role:      llm.MessageRole(resp.Role),
				Content:   fullText,
				Items:     items,
				ToolCalls: toolCalls,
			},
			FinishReason: resp.StopReason,
		}},
		Usage: usage,
		Model: resp.Model,
	}
}

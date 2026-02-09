package claude

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/llm"
)

// ToRequest converts an llm.ChatRequest to a Claude API Request
func ToRequest(ctx context.Context, request *llm.GenerateRequest) (*Request, error) {
	req := &Request{
		AnthropicVersion: "bedrock-2023-05-31",
	}

	// Set options if provided
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
	}

	if request.Options != nil && len(request.Options.Tools) > 0 {
		for _, tool := range request.Options.Tools {
			// If caller supplied a full JSON schema use it as-is; otherwise wrap parameters.
			var inputSchema map[string]interface{}
			if tool.Definition.Parameters != nil {
				// assume already structured; ensure required is at top-level
				if _, hasType := tool.Definition.Parameters["type"]; hasType {
					// treat as full schema
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

			def := ToolDefinition{
				Name:        tool.Definition.Name,
				Description: tool.Definition.Description,
				InputSchema: inputSchema,
			}
			if len(tool.Definition.OutputSchema) > 0 {
				//def.OutputSchema = tool.Definition.OutputSchema
			}
			req.Tools = append(req.Tools, def)
		}
	}
	builder := strings.Builder{}
	firstAdded := false
	docNr := 1

	if strings.TrimSpace(request.Instructions) != "" {
		builder.WriteString(strings.TrimSpace(request.Instructions))
		firstAdded = true
	}

	// Find system message
	for _, msg := range request.Messages {
		if msg.Role != llm.RoleSystem {
			continue
		}

		if !firstAdded {
			builder.WriteString(llm.MessageText(msg))
			firstAdded = true
		} else {
			builder.WriteString(fmt.Sprintf("\n\n## Document %d:\n", docNr))
			builder.WriteString(llm.MessageText(msg))
			docNr++
		}
	}
	req.System = builder.String()

	// Convert messages
	for _, msg := range request.Messages {

		// Skip system messages as they're handled separately
		if msg.Role == llm.RoleSystem {
			continue
		}

		// Tool invocations requested by the assistant ---------------------------
		if len(msg.ToolCalls) > 0 {
			var useBlocks []ContentBlock
			for _, tc := range msg.ToolCalls {
				// Ensure an ID is present (required by Bedrock)
				id := tc.ID
				if id == "" {
					id = tc.Name // fallback; ideally caller provides a stable ID
				}
				useBlocks = append(useBlocks, ContentBlock{
					Type:  "tool_use",
					ID:    id,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			req.Messages = append(req.Messages, Message{Role: string(msg.Role), Content: useBlocks})
			continue
		}

		// Tool results from the caller ----------------------------------------
		if msg.Role == llm.RoleTool && msg.ToolCallId != "" {
			// Bedrock expects tool result as role "user"
			var resultContent interface{}
			if len(msg.Items) > 0 {
				resultContent = msg.Items[0].Data
			} else if msg.Content != "" {
				resultContent = msg.Content
			}
			req.Messages = append(req.Messages, Message{
				Role: "user",
				Content: []ContentBlock{{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallId,
					Content:   resultContent,
				}},
			})
			continue
		}

		claudeMsg := Message{
			Role: string(msg.Role),
		}

		// Convert content items
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
				contentBlock, err := handleImageContent(ctx, afs.New(), item)
				if err != nil {
					return nil, err
				}
				claudeMsg.Content = append(claudeMsg.Content, *contentBlock)
			default:
				return nil, fmt.Errorf("unsupported content type: %s", item.Type)
			}
		}

		// If no content items but there's content text, add it as a text content block
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

	// Inject cache_control into the penultimate message's last content block
	// as required for Bedrock provider. We do this only if there are at least
	// two messages and the penultimate message has at least one content block.
	if len(req.Messages) >= 2 {
		penIdx := len(req.Messages) - 2
		pen := req.Messages[penIdx]
		if len(pen.Content) > 0 {
			lastIdx := len(pen.Content) - 1
			// Set cache_control on the existing last content element without adding new blocks
			if pen.Content[lastIdx].CacheControl == nil {
				cc := &CacheControl{Type: "ephemeral"}
				pen.Content[lastIdx].CacheControl = cc
				// write back the modified message
				req.Messages[penIdx] = pen
			}
		}
	}

	return req, nil
}

// handleImageContent processes an image content item
func handleImageContent(ctx context.Context, fs afs.Service, item llm.ContentItem) (*ContentBlock, error) {
	var imageData string
	var mediaType string

	// Handle different image sources
	switch item.Source {
	case llm.SourceURL:
		// URL source not supported for Claude API
		return nil, fmt.Errorf("URL source not supported for Claude API, use base64 encoding instead")
	case llm.SourceBase64:
		// For base64 sources, use the data directly
		imageData = item.Data
		mediaType = item.MimeType
	case llm.SourceRaw:
		// Raw data needs to be base64 encoded
		mediaType = item.MimeType
		if mediaType == "" {
			mediaType = "image/png" // Default mime type
		}
		imageData = base64.StdEncoding.EncodeToString([]byte(item.Data))
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

// ToLLMSResponse converts a Claude API Response to an llm.ChatResponse
func ToLLMSResponse(resp *Response) *llm.GenerateResponse {
	var fullText string
	var items []llm.ContentItem
	var toolCalls []llm.ToolCall

	for _, item := range resp.Content {
		switch item.Type {
		case "text":
			fullText += item.Text
			items = append(items, llm.ContentItem{
				Type:   llm.ContentTypeText,
				Source: llm.SourceRaw,
				Data:   item.Text,
				Text:   item.Text,
			})
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        item.ID,
				Name:      item.Name,
				Arguments: item.Input,
			})
		}
	}

	msg := llm.Message{
		Role:      llm.RoleAssistant,
		Content:   fullText,
		Items:     items,
		ToolCalls: toolCalls,
	}

	return &llm.GenerateResponse{
		Choices: []llm.Choice{{
			Index:        0,
			Message:      msg,
			FinishReason: resp.StopReason,
		}},
		Usage: &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
		Model: resp.Model,
	}
}

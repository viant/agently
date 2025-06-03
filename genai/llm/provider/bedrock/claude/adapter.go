package claude

import (
	"context"
	"encoding/base64"
	"fmt"
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
		req.MaxTokens = request.Options.MaxTokens
		req.Temperature = request.Options.Temperature
		req.TopP = request.Options.TopP
	}

	if request.Options != nil && len(request.Options.Tools) > 0 {
		var cfg ToolConfig
		for _, tool := range request.Options.Tools {
			inputSchema := map[string]interface{}{
				"type":       "object",
				"properties": tool.Definition.Parameters,
			}
			if len(tool.Definition.Required) > 0 {
				inputSchema["required"] = tool.Definition.Required
			}
			def := ToolDefinition{
				Name:        tool.Definition.Name,
				Description: tool.Definition.Description,
				InputSchema: inputSchema,
			}
			if len(tool.Definition.OutputSchema) > 0 {
				def.OutputSchema = tool.Definition.OutputSchema
			}
			cfg.Tools = append(cfg.Tools, def)
		}
		req.ToolConfig = &cfg
	}

	// Find system message
	for _, msg := range request.Messages {
		if msg.Role == llm.RoleSystem {
			req.System = msg.Content
			break
		}
	}

	// Convert messages
	for _, msg := range request.Messages {

		// Skip system messages as they're handled separately
		if msg.Role == llm.RoleSystem {
			continue
		}

		if len(msg.ToolCalls) > 0 {
			var useBlocks []ContentBlock
			for _, tc := range msg.ToolCalls {
				useBlocks = append(useBlocks, ContentBlock{
					ToolUse: &ToolUseBlock{
						ToolUseId: tc.ID,
						Name:      tc.Name,
						Input:     tc.Arguments,
					},
				})
			}
			req.Messages = append(req.Messages, Message{Role: string(msg.Role), Content: useBlocks})
			continue
		}

		if msg.Role == llm.RoleTool && msg.ToolCallId != "" {
			var resultBlocks []ToolResultContentBlock
			if len(msg.Items) > 0 {
				for _, item := range msg.Items {
					text := item.Data
					resultBlocks = append(resultBlocks, ToolResultContentBlock{Text: &text})
				}
			} else if msg.Content != "" {
				text := msg.Content
				resultBlocks = append(resultBlocks, ToolResultContentBlock{Text: &text})
			}
			req.Messages = append(req.Messages, Message{Role: string(msg.Role), Content: []ContentBlock{{ToolResult: &ToolResultBlock{ToolUseId: msg.ToolCallId, Content: resultBlocks}}}})
			continue
		}

		claudeMsg := Message{
			Role: string(msg.Role),
		}

		// Convert content items
		for _, item := range msg.Items {
			switch item.Type {
			case llm.ContentTypeText:
				claudeMsg.Content = append(claudeMsg.Content, ContentBlock{
					Type: "text",
					Text: item.Text,
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
			claudeMsg.Content = append(claudeMsg.Content, ContentBlock{
				Type: "text",
				Text: msg.Content,
			})
		}

		req.Messages = append(req.Messages, claudeMsg)
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
	var contentItems []llm.ContentItem

	// Extract text from content items
	for _, item := range resp.Content {
		if item.Type == "text" {
			fullText += item.Text
			contentItems = append(contentItems, llm.ContentItem{
				Type:   llm.ContentTypeText,
				Source: llm.SourceRaw,
				Data:   item.Text,
				Text:   item.Text,
			})
		}
	}

	// Create the response
	return &llm.GenerateResponse{
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: fullText,
					Items:   contentItems,
				},
				FinishReason: resp.StopReason,
			},
		},
		Usage: &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
		Model: resp.Model,
	}
}

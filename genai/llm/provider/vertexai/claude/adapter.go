package claude

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/viant/afs"
	"github.com/viant/agently/genai/llm"
)

// ToRequest converts a generic llm.ChatRequest to a Claude-specific Request
func ToRequest(ctx context.Context, request *llm.GenerateRequest) (*Request, error) {
	if request == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	claudeReq := &Request{
		AnthropicVersion: defaultAnthropicVersion,
		MaxTokens:        256, // Default value
	}

	// Set options if provided
	if opts := request.Options; opts != nil {
		if request.Options.MaxTokens > 0 {
			claudeReq.MaxTokens = opts.MaxTokens
		}

		if thinking := opts.Thinking; thinking != nil {
			// Add thinking by default
			claudeReq.Thinking = &Thinking{
				Type:         thinking.Type,
				BudgetTokens: max(thinking.BudgetTokens, 1024), // Default value
			}
		}
		claudeReq.Stream = opts.Stream
	}

	// Convert messages
	claudeMessages := make([]Message, 0, len(request.Messages))

	fs := afs.New()

	for _, msg := range request.Messages {

		if msg.Role == llm.RoleSystem {
			claudeReq.System = msg.Content
			continue
		}
		// Handle assistant tool calls as toolUse blocks
		if len(msg.ToolCalls) > 0 {
			var blocks []ContentBlock
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, ContentBlock{
					ToolUse: &ToolUseBlock{
						ToolUseId: tc.ID,
						Name:      tc.Name,
						Input:     tc.Arguments,
					},
				})
			}
			claudeMessages = append(claudeMessages, Message{
				Role:    string(msg.Role),
				Content: blocks,
			})
			continue
		}
		// Handle tool result messages as toolResult blocks
		if msg.Role == llm.RoleTool && msg.ToolCallId != "" {
			var blocks []ContentBlock
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
			blocks = append(blocks, ContentBlock{
				ToolResult: &ToolResultBlock{
					ToolUseId: msg.ToolCallId,
					Content:   resultBlocks,
				},
			})
			claudeMessages = append(claudeMessages, Message{
				Role:    string(msg.Role),
				Content: blocks,
			})
			continue
		}

		claudeMessage := Message{
			Role:    string(msg.Role),
			Content: []ContentBlock{},
		}

		// Handle content items
		for _, item := range msg.Items {
			switch item.Type {
			case llm.ContentTypeText:
				claudeMessage.Content = append(claudeMessage.Content, ContentBlock{
					Type: "text",
					Text: item.Data,
				})
			case llm.ContentTypeImage:
				contentBlock, err := handleImageContent(ctx, fs, item)
				if err != nil {
					return nil, err
				}
				claudeMessage.Content = append(claudeMessage.Content, *contentBlock)
			default:
				// Skip unsupported content types
				continue
			}
		}

		claudeMessages = append(claudeMessages, claudeMessage)
	}

	claudeReq.Messages = claudeMessages
	return claudeReq, nil
}

// handleImageContent processes image content items and converts them to Claude format
func handleImageContent(ctx context.Context, fs afs.Service, item llm.ContentItem) (*ContentBlock, error) {
	var mimeType string
	var imageData string

	switch item.Source {
	case llm.SourceBase64:
		// Already base64 encoded
		mimeType = item.MimeType
		if mimeType == "" {
			mimeType = "image/png" // Default mime type
		}
		imageData = item.Data
	case llm.SourceURL:
		// For URLs, we'll just pass the URL directly and let the client handle it
		return nil, fmt.Errorf("URL source not supported for Claude API, use base64 encoding instead")
	case llm.SourceRaw:
		// Raw data needs to be base64 encoded
		mimeType = item.MimeType
		if mimeType == "" {
			mimeType = "image/png" // Default mime type
		}
		imageData = base64.StdEncoding.EncodeToString([]byte(item.Data))
	default:
		return nil, fmt.Errorf("unsupported source type: %s", item.Source)
	}

	return &ContentBlock{
		Type: "image",
		Source: &Source{
			Type:      "base64",
			MediaType: mimeType,
			Data:      imageData,
		},
	}, nil
}

// ToLLMSResponse converts a Claude-specific Response to a generic llm.ChatResponse
func ToLLMSResponse(resp *Response) *llm.GenerateResponse {
	if resp == nil {
		return &llm.GenerateResponse{
			Choices: []llm.Choice{},
		}
	}

	// Check if this is a VertexAI Claude response format
	if resp.ID != "" && resp.Content != nil {
		return handleVertexAIResponse(resp)
	}

	// Handle different response types
	if resp.Type == "message" && resp.Message.Role != "" {
		// Extract text content from the message
		var content string
		for _, block := range resp.Message.Content {
			if block.Type == "text" {
				content += block.Text
			}
		}

		return &llm.GenerateResponse{
			Choices: []llm.Choice{
				{
					Index: 0,
					Message: llm.Message{
						Role:    llm.MessageRole(resp.Message.Role),
						Content: content,
						Items: []llm.ContentItem{
							{
								Type:   llm.ContentTypeText,
								Source: llm.SourceRaw,
								Data:   content,
								Text:   content,
							},
						},
					},
					FinishReason: "stop",
				},
			},
		}
	} else if resp.Type == "message_delta" && resp.Delta != nil {
		// Handle streaming delta response
		return &llm.GenerateResponse{
			Choices: []llm.Choice{
				{
					Index: 0,
					Message: llm.Message{
						Role:    llm.RoleAssistant,
						Content: resp.Delta.Text,
						Items: []llm.ContentItem{
							{
								Type:   llm.ContentTypeText,
								Source: llm.SourceRaw,
								Data:   resp.Delta.Text,
								Text:   resp.Delta.Text,
							},
						},
					},
					FinishReason: resp.Delta.StopReason,
				},
			},
		}
	} else if resp.Type == "error" && resp.Error != nil {
		// Handle error response
		errorMsg := resp.Error.Message
		return &llm.GenerateResponse{
			Choices: []llm.Choice{
				{
					Index: 0,
					Message: llm.Message{
						Role:    llm.RoleAssistant,
						Content: "Error: " + errorMsg,
						Items: []llm.ContentItem{
							{
								Type:   llm.ContentTypeText,
								Source: llm.SourceRaw,
								Data:   "Error: " + errorMsg,
								Text:   "Error: " + errorMsg,
							},
						},
					},
					FinishReason: "error",
				},
			},
		}
	}

	// Default empty response
	return &llm.GenerateResponse{
		Choices: []llm.Choice{},
	}
}

// handleVertexAIResponse converts a VertexAI Claude response to a generic llm.ChatResponse
func handleVertexAIResponse(resp *Response) *llm.GenerateResponse {
	// Extract text content from the response
	var content string

	// Handle content which could be a mix of different types
	for _, item := range resp.Content {
		// Try to convert to map to extract text
		contentMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		contentType, ok := contentMap["type"].(string)
		if !ok || contentType != "text" {
			continue
		}

		text, ok := contentMap["text"].(string)
		if ok {
			content += text
		}
	}

	// Create usage information if available
	var usage *llm.Usage
	if resp.Usage != nil {
		usage = &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return &llm.GenerateResponse{
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.MessageRole(resp.Role),
					Content: content,
					Items: []llm.ContentItem{
						{
							Type:   llm.ContentTypeText,
							Source: llm.SourceRaw,
							Data:   content,
							Text:   content,
						},
					},
				},
				FinishReason: resp.StopReason,
			},
		},
		Usage: usage,
		Model: resp.Model,
	}
}

// VertexAIResponseToLLMS converts a VertexAI Claude response to a generic llm.ChatResponse
func VertexAIResponseToLLMS(resp *VertexAIResponse) *llm.GenerateResponse {
	if resp == nil {
		return &llm.GenerateResponse{
			Choices: []llm.Choice{},
		}
	}

	// Extract text content from the response
	var content string
	for _, item := range resp.Content {
		if item.Type == "text" {
			content += item.Text
		}
	}

	// Create usage information if available
	var usage *llm.Usage
	if resp.Usage != nil {
		usage = &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}

	return &llm.GenerateResponse{
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.MessageRole(resp.Role),
					Content: content,
					Items: []llm.ContentItem{
						{
							Type:   llm.ContentTypeText,
							Source: llm.SourceRaw,
							Data:   content,
							Text:   content,
						},
					},
				},
				FinishReason: resp.StopReason,
			},
		},
		Usage: usage,
		Model: resp.Model,
	}
}

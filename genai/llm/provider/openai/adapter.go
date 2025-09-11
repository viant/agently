package openai

import (
	"github.com/viant/agently/genai/llm"
)

var modelTemperature = map[string]float64{
	"o4-mini": 1.0,
	"o1-mini": 1.0,
	"o3-mini": 1.0,
	"o3":      1.0,
}

// ToRequest converts an llm.ChatRequest to a Request
func ToRequest(request *llm.GenerateRequest) *Request {
	// Create the request with default values
	req := &Request{}

	// Set options if provided
	if request.Options != nil {
		// Set model if provided
		if request.Options.Model != "" {
			req.Model = request.Options.Model
		}

		// Set max tokens if provided
		if request.Options.MaxTokens > 0 {
			req.MaxTokens = request.Options.MaxTokens
		}

		// Set top_p if provided
		if request.Options.TopP > 0 {
			req.TopP = request.Options.TopP
		}

		// Set temperature only when explicitly specified (>0)
		if request.Options.Temperature > 0 {
			req.Temperature = &request.Options.Temperature
		}

		// Set n if provided
		if request.Options.N > 0 {
			req.N = request.Options.N
		}
		// Enable streaming if requested
		req.Stream = request.Options.Stream
		// Propagate reasoning summary if requested on supported models
		if r := request.Options.Reasoning; r != nil && r.Summary == "auto" {
			switch req.Model {
			case "o3", "o4-mini", "codex-mini-latest":
				req.Reasoning = r
			}
		}

		// Convert tools if provided
		if len(request.Options.Tools) > 0 {
			req.Tools = make([]Tool, len(request.Options.Tools))
			for i, tool := range request.Options.Tools {
				req.Tools[i] = Tool{
					Type: "function",
					Function: ToolDefinition{
						Name:        tool.Definition.Name,
						Description: tool.Definition.Description,
						Parameters:  tool.Definition.Parameters,
						Required:    tool.Definition.Required,
					},
				}
			}
		}

		// Convert tool choice if provided
		if request.Options.ToolChoice.Type != "" {
			switch request.Options.ToolChoice.Type {
			case "auto":
				req.ToolChoice = "auto"
			case "none":
				req.ToolChoice = "none"
			case "function":
				if request.Options.ToolChoice.Function != nil {
					req.ToolChoice = map[string]interface{}{
						"type": "function",
						"function": map[string]string{
							"name": request.Options.ToolChoice.Function.Name,
						},
					}
				}
			}
		}
	}

	// Convert messages
	req.Messages = make([]Message, len(request.Messages))
	for i, msg := range request.Messages {
		message := Message{
			Role: string(msg.Role),
			Name: msg.Name,
		}

		// Handle content based on priority: Items > ContentItems > Result
		if len(msg.Items) > 0 {
			// Convert Items to OpenAI format
			contentItems := make([]ContentItem, len(msg.Items))
			for j, item := range msg.Items {
				contentItem := ContentItem{
					Type: string(item.Type),
				}

				// Handle different content types
				switch item.Type {
				case llm.ContentTypeText:
					// Use Data field first, fall back to Text field
					if item.Data != "" {
						contentItem.Text = item.Data
					} else {
						contentItem.Text = item.Text
					}
				case llm.ContentTypeImage, llm.ContentTypeImageURL:
					// OpenAI expects "image_url" as the type
					contentItem.Type = "image_url"

					// Preferred approach: Use Source=SourceURL and Data field
					if item.Source == llm.SourceURL && item.Data != "" {
						contentItem.ImageURL = &ImageURL{
							URL: item.Data,
						}

						// Add detail if available in metadata
						if item.Metadata != nil {
							if detail, ok := item.Metadata["detail"].(string); ok {
								contentItem.ImageURL.Detail = detail
							}
						}
					}
				}

				contentItems[j] = contentItem
			}
			message.Content = contentItems
		} else if len(msg.ContentItems) > 0 {
			// Legacy: Convert ContentItems to OpenAI format
			contentItems := make([]ContentItem, len(msg.ContentItems))
			for j, item := range msg.ContentItems {
				contentItem := ContentItem{
					Type: string(item.Type),
				}

				if item.Type == llm.ContentTypeText {
					// Use Data field first, fall back to Text field
					if item.Data != "" {
						contentItem.Text = item.Data
					} else {
						contentItem.Text = item.Text
					}
				} else if item.Type == llm.ContentTypeImage || item.Type == llm.ContentTypeImageURL {
					// OpenAI expects "image_url" as the type
					contentItem.Type = "image_url"

					// Preferred approach: Use Source=SourceURL and Data field
					if item.Source == llm.SourceURL && item.Data != "" {
						contentItem.ImageURL = &ImageURL{
							URL: item.Data,
						}

						// Add detail if available in metadata
						if item.Metadata != nil {
							if detail, ok := item.Metadata["detail"].(string); ok {
								contentItem.ImageURL.Detail = detail
							}
						}
					}
				}

				contentItems[j] = contentItem
			}
			message.Content = contentItems
		} else if msg.Content != "" {
			// Use simple string content for backward compatibility
			message.Content = msg.Content
		}

		// Convert function call if present
		if msg.FunctionCall != nil {
			message.FunctionCall = &FunctionCall{
				Name:      msg.FunctionCall.Name,
				Arguments: msg.FunctionCall.Arguments,
			}
		}

		message.ToolCallId = msg.ToolCallId
		// Convert tool calls if present
		if len(msg.ToolCalls) > 0 {
			message.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
			for j, toolCall := range msg.ToolCalls {
				message.ToolCalls[j] = ToolCall{
					ID:   toolCall.ID,
					Type: toolCall.Type,
					Function: FunctionCall{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
			}
		}

		req.Messages[i] = message
	}

	return req
}

// ToLLMSResponse converts a Response to an llm.ChatResponse
func ToLLMSResponse(resp *Response) *llm.GenerateResponse {
	// Create the LLMS response
	llmsResp := &llm.GenerateResponse{
		Choices: make([]llm.Choice, len(resp.Choices)),
	}

	// Convert choices
	for i, choice := range resp.Choices {
		llmsChoice := llm.Choice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
		}

		// Create the message with basic fields
		message := llm.Message{
			Role: llm.MessageRole(choice.Message.Role),
			Name: choice.Message.Name,
		}

		// Handle content based on its type
		switch content := choice.Message.Content.(type) {
		case string:
			// Simple string content
			message.Content = content
		case []ContentItem:
			// Convert content items to internal format
			message.ContentItems = make([]llm.ContentItem, len(content))
			for j, item := range content {
				contentItem := llm.ContentItem{
					Type: llm.ContentType(item.Type),
				}

				if item.Type == "text" {
					contentItem.Text = item.Text
					contentItem.Source = llm.SourceRaw
					contentItem.Data = item.Text
				} else if item.Type == "image_url" && item.ImageURL != nil {
					// Set the proper content type
					contentItem.Type = llm.ContentTypeImage

					// Use the preferred approach: Source=SourceURL and Data field
					contentItem.Source = llm.SourceURL
					contentItem.Data = item.ImageURL.URL

					// Add detail to metadata if present
					if item.ImageURL.Detail != "" {
						if contentItem.Metadata == nil {
							contentItem.Metadata = make(map[string]interface{})
						}
						contentItem.Metadata["detail"] = item.ImageURL.Detail
					}

				}

				message.ContentItems[j] = contentItem
			}
		case []interface{}:
			// Handle case where content is a generic slice
			message.ContentItems = make([]llm.ContentItem, 0, len(content))
			for _, item := range content {
				if itemMap, ok := item.(map[string]interface{}); ok {
					contentType, _ := itemMap["type"].(string)
					contentItem := llm.ContentItem{
						Type: llm.ContentType(contentType),
					}

					if contentType == "text" {
						text, _ := itemMap["text"].(string)
						contentItem.Text = text
						contentItem.Source = llm.SourceRaw
						contentItem.Data = text
					} else if contentType == "image_url" {
						if imageURL, ok := itemMap["image_url"].(map[string]interface{}); ok {
							url, _ := imageURL["url"].(string)
							detail, _ := imageURL["detail"].(string)

							// Set the proper content type
							contentItem.Type = llm.ContentTypeImage

							// Use the preferred approach: Source=SourceURL and Data field
							contentItem.Source = llm.SourceURL
							contentItem.Data = url

							// Add detail to metadata if present
							if detail != "" {
								if contentItem.Metadata == nil {
									contentItem.Metadata = make(map[string]interface{})
								}
								contentItem.Metadata["detail"] = detail
							}
						}
					}

					message.ContentItems = append(message.ContentItems, contentItem)
				}
			}
		}

		// Convert function call if present
		if choice.Message.FunctionCall != nil {
			message.FunctionCall = &llm.FunctionCall{
				Name:      choice.Message.FunctionCall.Name,
				Arguments: choice.Message.FunctionCall.Arguments,
			}
		}

		// Convert tool calls if present
		if len(choice.Message.ToolCalls) > 0 {
			message.ToolCalls = make([]llm.ToolCall, len(choice.Message.ToolCalls))
			for j, toolCall := range choice.Message.ToolCalls {
				message.ToolCalls[j] = llm.ToolCall{
					ID:   toolCall.ID,
					Type: toolCall.Type,
					Function: llm.FunctionCall{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
			}
		}
		// Preserve tool call result ID if present
		message.ToolCallId = choice.Message.ToolCallId

		llmsChoice.Message = message
		llmsResp.Choices[i] = llmsChoice
	}

	// Convert usage
	llmsResp.Usage = &llm.Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}

	return llmsResp
}

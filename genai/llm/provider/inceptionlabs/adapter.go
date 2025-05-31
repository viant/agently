package inceptionlabs

import (
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	"strings"
)

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

		if request.Options.Temperature >= 0 {
			// Copy temperature (omit when default 1)
			req.Temperature = &request.Options.Temperature
		}
		// Set max tokens if provided
		if request.Options.MaxTokens > 0 {
			req.MaxTokens = request.Options.MaxTokens
		}

		// Set top_p if provided
		if request.Options.TopP > 0 {
			req.TopP = request.Options.TopP
		}

		// Set n if provided
		if request.Options.N > 0 {
			req.N = request.Options.N
		}
	}

	// Build messages slice, with optional tool definitions system prompt
	var messages []Message
	if request.Options != nil && len(request.Options.Tools) > 0 {
		// List available tools in system prompt
		var desc strings.Builder
		desc.WriteString("Available tools:\n")
		for _, t := range request.Options.Tools {
			desc.WriteString(fmt.Sprintf("- %s: %s\n", t.Definition.Name, t.Definition.Description))
			if props, ok := t.Definition.Parameters["properties"]; ok {
				if aMap := props.(map[string]interface{}); aMap != nil {
					if p, err := json.Marshal(props); err == nil {
						props = string(p)
					}
				}
				desc.WriteString(fmt.Sprintf("  parameters: %v\n", props))
			}
		}
		messages = append(messages, Message{Role: string(llm.RoleSystem), Content: desc.String()})
	}
	// Convert original messages
	messages = append(messages, make([]Message, 0)...) // ensure capacity
	for _, msg := range request.Messages {
		message := Message{
			Role: string(msg.Role),
			Name: msg.Name,
		}

		// Handle content based on priority: Items > ContentItems > Result
		if len(msg.Items) > 0 {
			// For InceptionLabs, we can only send text content
			// Combine all text items into a single content string
			var content string
			for _, item := range msg.Items {
				if item.Type == llm.ContentTypeText {
					if item.Data != "" {
						content += item.Data + "\n"
					} else {
						content += item.Text + "\n"
					}
				}
			}
			message.Content = content
		} else if len(msg.ContentItems) > 0 {
			// Legacy: Combine all text items into a single content string
			var content string
			for _, item := range msg.ContentItems {
				if item.Type == llm.ContentTypeText {
					if item.Data != "" {
						content += item.Data + "\n"
					} else {
						content += item.Text + "\n"
					}
				}
			}
			message.Content = content
		} else if msg.Content != "" {
			// Use simple string content for backward compatibility
			message.Content = msg.Content
		}

		// Result conversion logic unchanged
		if len(msg.Items) > 0 {
			var content string
			for _, item := range msg.Items {
				if item.Type == llm.ContentTypeText {
					if item.Data != "" {
						content += item.Data + "\n"
					} else {
						content += item.Text + "\n"
					}
				}
			}
			message.Content = content
		} else if len(msg.ContentItems) > 0 {
			var content string
			for _, item := range msg.ContentItems {
				if item.Type == llm.ContentTypeText {
					if item.Data != "" {
						content += item.Data + "\n"
					} else {
						content += item.Text + "\n"
					}
				}
			}
			message.Content = content
		} else if msg.Content != "" {
			message.Content = msg.Content
		}
		messages = append(messages, message)
	}
	req.Messages = messages
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
			Role:    llm.MessageRole(choice.Message.Role),
			Name:    choice.Message.Name,
			Content: choice.Message.Content,
		}

		// Create a content item for the text
		textItem := llm.ContentItem{
			Type:   llm.ContentTypeText,
			Source: llm.SourceRaw,
			Data:   choice.Message.Content,
			Text:   choice.Message.Content,
		}

		// Add the content item to the message
		message.Items = []llm.ContentItem{textItem}

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

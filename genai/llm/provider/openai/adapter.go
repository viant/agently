package openai

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/viant/afs/option/content"
	"github.com/viant/afs/storage"
	"github.com/viant/afsc/openai/assets"
	"github.com/viant/agently/internal/shared"

	openai "github.com/openai/openai-go/v3"
	"github.com/viant/agently/genai/llm"
	authctx "github.com/viant/agently/internal/auth"
	mcpname "github.com/viant/agently/pkg/mcpname"
)

var modelTemperature = map[string]float64{
	"o4-mini": 1.0,
	"o1-mini": 1.0,
	"o3-mini": 1.0,
	"o3":      1.0,
}

// ToRequest converts an llm.ChatRequest to a Request
func (c *Client) ToRequest(request *llm.GenerateRequest) (*Request, error) {
	// Create the request with defaults
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
		if r := request.Options.Reasoning; r != nil {
			switch req.Model {
			case "o3", "o4-mini", "codex-mini-latest",
				"gpt-4.1", "gpt-4.1-mini", "gpt-5", "o3-mini":
				req.Reasoning = r
			}
		}

		// Convert tools if provided
		if len(request.Options.Tools) > 0 {
			req.Tools = make([]Tool, len(request.Options.Tools))
			for i, tool := range request.Options.Tools {
				def := tool.Definition
				def.Normalize() // ensure provider-agnostic, valid JSON schema shapes
				req.Tools[i] = Tool{
					Type: "function",
					Function: ToolDefinition{
						Name:        mcpname.Canonical(def.Name),
						Description: def.Description,
						Parameters:  def.Parameters,
						Required:    def.Required,
						Strict:      def.Strict,
					},
				}
			}
		}

		// Honor parallel tool calls option (agent-configurable, provider-supported)
		if request.Options.ParallelToolCalls {
			req.ParallelToolCalls = true
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

	// Attachment preferences and limits
	attachMode := "upload" // prefer upload for tool-result PDFs
	agentID := "unknownAgent"
	var ttlSec int64
	// default threshold ~200kB for converting tool results to PDF attachments
	if request != nil && request.Options != nil && request.Options.Metadata != nil {
		if v, ok := request.Options.Metadata["attachMode"].(string); ok && strings.TrimSpace(v) != "" {
			attachMode = strings.ToLower(strings.TrimSpace(v))
		}
		if v, ok := request.Options.Metadata["agentId"].(string); ok && strings.TrimSpace(v) != "" {
			agentID = strings.ToLower(strings.TrimSpace(v))
		}
		if v, ok := request.Options.Metadata["attachmentTTLSec"]; ok {
			switch t := v.(type) {
			case int:
				ttlSec = int64(t)
			case int64:
				ttlSec = t
			case float64:
				ttlSec = int64(t)
			case string:
				if n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
					ttlSec = n
				}
			}
		}
	}
	ToolCallIdToReplaceContent := map[string]struct{}{}

	// Continue previous Responses API call when requested
	if request != nil && strings.TrimSpace(request.PreviousResponseID) != "" {
		req.PreviousResponseID = strings.TrimSpace(request.PreviousResponseID)
	}

	// Convert messages
	req.Messages = make([]Message, 0) //len(request.Messages))
	for _, originalMsg := range request.Messages {
		// Work on a local copy so we can transform tool messages if needed.
		msg := originalMsg
		message := Message{
			Role: string(msg.Role),
		}
		// Propagate speaker name only for user/assistant roles
		if msg.Role == llm.RoleUser || msg.Role == llm.RoleAssistant {
			message.Name = msg.Name
		}

		// Optionally convert large tool results into PDF attachments
		if msg.Role == llm.RoleTool {
			// Aggregate text payload length from Items/Content/ContentItems
			var textPayload string
			if len(msg.Items) > 0 {
				for _, it := range msg.Items {
					if it.Type == llm.ContentTypeText && it.Data != "" {
						textPayload += it.Data
					} else if it.Type == llm.ContentTypeText && it.Text != "" {
						textPayload += it.Text
					}
				}
			} else if msg.Content != "" {
				textPayload = msg.Content
			} else if len(msg.ContentItems) > 0 {
				for _, it := range msg.ContentItems {
					if it.Type == llm.ContentTypeText && it.Data != "" {
						textPayload += it.Data
					} else if it.Type == llm.ContentTypeText && it.Text != "" {
						textPayload += it.Text
					}
				}
			}

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
				case llm.ContentTypeBinary:
					if attachMode == "inline" {
						if strings.HasPrefix(item.MimeType, "image/") && item.Data != "" {
							// For images, inline as data URL to support vision (OpenAI expects image_url)
							contentItem.Type = "image_url"
							dataURL := "data:" + item.MimeType + ";base64," + item.Data
							contentItem.ImageURL = &ImageURL{URL: dataURL}
						} else if strings.EqualFold(item.MimeType, "application/pdf") && item.Data != "" {
							contentItem.Type = "file"
							contentItem.File = &File{
								FileName: item.Name,
								FileData: "data:" + item.MimeType + ";base64," + item.Data,
							}
							//return nil, fmt.Errorf("unsupported binary content item (mimeType: %v)", item.MimeType)
						} else { //TODO return error
							contentItem.Type = "file"
							contentItem.File = &File{
								FileName: item.Name,
								FileData: "data:" + item.MimeType + ";base64," + item.Data,
							}
						}
					} else { // TODO only for pdf, return error otherwise
						if strings.HasPrefix(item.MimeType, "image/") && item.Data != "" {
							// For images, inline as data URL to support vision (OpenAI expects image_url)
							contentItem.Type = "image_url"
							dataURL := "data:" + item.MimeType + ";base64," + item.Data
							contentItem.ImageURL = &ImageURL{URL: dataURL}
						} else { // TODO return error if not pdf
							fileID, err := c.uploadFiledAndGetID(context.Background(), item.Data, item.Name, agentID, ttlSec)
							if err != nil {
								return nil, fmt.Errorf("failed to upload PDF content item: %w", err)
							}

							_, ok := ToolCallIdToReplaceContent[msg.ToolCallId]
							if msg.Role == "tool" && ok {
								msg.Content = fmt.Sprintf(`{"status":"ok","file_id":"%s","filename":"%s","bytes":%d}`, fileID, item.Name, len(item.Data))
								msg.Items = []llm.ContentItem{}
							} else {
								contentItem.Type = "file"
								contentItem.File = &File{FileID: fileID}
							}

						}
					}
				}
				contentItems[j] = contentItem
			}
			if _, ok := ToolCallIdToReplaceContent[msg.ToolCallId]; !ok {
				message.Content = contentItems
			} else {
				message.Content = msg.Content
			}
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
					Type: "function",

					Function: FunctionCall{
						Name:      toolCall.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
			}
		}

		req.Messages = append(req.Messages, message)
		//req.Messages[i] = message
	}

	return req, nil
}

// ToRequest is a convenience wrapper retained for backward-compatible tests.
// It constructs a default client and adapts an llm.GenerateRequest to provider Request.
// Errors are ignored in this wrapper; callers requiring error handling should use Client.ToRequest.
func ToRequest(request *llm.GenerateRequest) *Request {
	c := &Client{}
	out, _ := c.ToRequest(request)
	return out
}

// uploadFiledAndGetID uploads a base64-encoded PDF to OpenAI assets and returns its file_id.
func (c *Client) uploadFiledAndGetID(ctx context.Context, base64Data string, name string, agentID string, ttlSec int64) (string, error) {
	// TODO detect duplicates, user

	var attachmentTTLSec int64 = ttlSec
	// Apply provider default TTL (86400 sec = 1 day) when not specified
	if attachmentTTLSec <= 0 {
		attachmentTTLSec = 86400
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err

	}

	user := ""
	if ui := authctx.User(ctx); ui != nil {
		user = strings.TrimSpace(ui.Subject)
		if user == "" {
			user = strings.TrimSpace(ui.Email)
		}
	}

	if user == "" {
		user, err = shared.PrefixHostIP()
	}
	if err != nil {
		return "", fmt.Errorf("failed to determine host ip prefix: %w", err)
	}

	filename := fmt.Sprintf("agently/%s/%s/%s/%s", user, agentID, c.Model, name)
	dest := "openai://assets/" + filename
	// Build options with optional TTL
	var opts []storage.Option
	opts = append(opts, &content.Meta{Values: map[string]string{"purpose": string(openai.FilePurposeUserData)}})
	// Always include TTL (provider default baked above)
	opts = append(opts, &openai.FileNewParamsExpiresAfter{Seconds: attachmentTTLSec})

	if err := c.storageMgr.Upload(ctx, dest, 0644, strings.NewReader(string(data)), opts...); err != nil {
		return "", err
	}

	// Find created file id by listing (with small retries)
	var fileID string
	for attempt := 0; attempt < 2 && fileID == ""; attempt++ {
		files, err := c.storageMgr.List(ctx, "openai://assets/")
		if err != nil {
			return "", err
		}
		for _, f := range files {
			if f.Name() == filename {
				if af, ok := f.Sys().(assets.File); ok {
					fileID = af.ID
					break
				}
			}
		}
		if fileID == "" {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if fileID == "" {
		return "", fmt.Errorf("uploaded file id not found")
	}
	return fileID, nil
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

	// Convert usage including detailed fields when available
	u := &llm.Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	// Map prompt details (prefer details; fallback to flattened field)
	u.PromptCachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	if u.PromptCachedTokens == 0 && resp.Usage.PromptCachedTokens > 0 {
		u.PromptCachedTokens = resp.Usage.PromptCachedTokens
	}
	if resp.Usage.PromptTokensDetails.AudioTokens > 0 {
		u.PromptAudioTokens = resp.Usage.PromptTokensDetails.AudioTokens
	}
	// Map completion details
	if resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
		u.ReasoningTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
		u.CompletionReasoningTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
	} else {
		// Fallback to flattened fields when details are absent
		if resp.Usage.CompletionReasoningTokens > 0 {
			u.CompletionReasoningTokens = resp.Usage.CompletionReasoningTokens
			if u.ReasoningTokens == 0 {
				u.ReasoningTokens = resp.Usage.CompletionReasoningTokens
			}
		} else if resp.Usage.ReasoningTokens > 0 && u.ReasoningTokens == 0 {
			u.ReasoningTokens = resp.Usage.ReasoningTokens
		}
	}
	if resp.Usage.CompletionTokensDetails.AudioTokens > 0 {
		u.CompletionAudioTokens = resp.Usage.CompletionTokensDetails.AudioTokens
		// Keep legacy aggregate when single source available
		if u.AudioTokens == 0 {
			u.AudioTokens = resp.Usage.CompletionTokensDetails.AudioTokens
		}
	}
	u.AcceptedPredictionTokens = resp.Usage.CompletionTokensDetails.AcceptedPredictionTokens
	u.RejectedPredictionTokens = resp.Usage.CompletionTokensDetails.RejectedPredictionTokens
	llmsResp.Usage = u

	return llmsResp
}

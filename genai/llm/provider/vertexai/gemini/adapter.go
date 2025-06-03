package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"mime"
	"path"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/afs/http"
	"github.com/viant/afs/url"
	"github.com/viant/agently/genai/llm"
)

// ToRequest converts an llm.ChatRequest to a Gemini Request
func ToRequest(ctx context.Context, request *llm.GenerateRequest) (*Request, error) {
	// Create the request with default values
	req := &Request{}

	fs := afs.New()
	// Convert messages to Gemini contents
	req.Contents = make([]Content, 0)

	// Set options if provided
	if request.Options != nil {
		// Propagate streaming flag if requested
		req.Stream = request.Options.Stream
		// Set generation config
		req.GenerationConfig = &GenerationConfig{}

		// Set temperature if provided
		if request.Options.Temperature > 0 {
			req.GenerationConfig.Temperature = request.Options.Temperature
		}

		// Set max tokens if provided
		if request.Options.MaxTokens > 0 {
			req.GenerationConfig.MaxOutputTokens = request.Options.MaxTokens
		}

		// Set top_p if provided
		if request.Options.TopP > 0 {
			req.GenerationConfig.TopP = request.Options.TopP
		}

		// Set top_k if provided
		if request.Options.TopK > 0 {
			req.GenerationConfig.TopK = request.Options.TopK
		}

		// Set candidate count if provided
		if request.Options.N > 0 {
			req.GenerationConfig.CandidateCount = request.Options.N
		}

		// Set presence penalty if provided
		if request.Options.PresencePenalty > 0 {
			req.GenerationConfig.PresencePenalty = request.Options.PresencePenalty
		}

		// Set frequency penalty if provided
		if request.Options.FrequencyPenalty > 0 {
			req.GenerationConfig.FrequencyPenalty = request.Options.FrequencyPenalty
		}

		// Set response MIME type if provided
		if request.Options.ResponseMIMEType != "" {
			req.GenerationConfig.ResponseMIMEType = request.Options.ResponseMIMEType
		}

		// Set seed if provided
		if request.Options.Seed > 0 {
			req.GenerationConfig.Seed = request.Options.Seed
		}

		// Set metadata if provided
		if request.Options.Metadata != nil {
			// Check if labels are provided in metadata
			if labels, ok := request.Options.Metadata["labels"].(map[string]string); ok {
				req.Labels = labels
			}
		}

		// Convert tools if provided
		if len(request.Options.Tools) > 0 {
			req.Tools = make([]Tool, 1)
			req.Tools[0].FunctionDeclarations = make([]FunctionDeclaration, len(request.Options.Tools))

			for i, tool := range request.Options.Tools {
				if tool.Type == "function" {
					req.Tools[0].FunctionDeclarations[i] = FunctionDeclaration{
						Name:        tool.Definition.Name,
						Description: tool.Definition.Description,
						Parameters:  tool.Definition.Parameters,
					}
				}
			}

			// Set tool config if tool choice is provided
			if request.Options.ToolChoice.Type != "" {
				req.ToolConfig = &ToolConfig{
					FunctionCallingConfig: &FunctionCallingConfig{},
				}

				switch request.Options.ToolChoice.Type {
				case "auto":
					req.ToolConfig.FunctionCallingConfig.Mode = "AUTO"
				case "none":
					req.ToolConfig.FunctionCallingConfig.Mode = "NONE"
				case "function":
					req.ToolConfig.FunctionCallingConfig.Mode = "ANY"
				}
			}
		}
	}

	for _, msg := range request.Messages {

		// Map roles from llms to Gemini
		role := ""
		switch msg.Role {
		case llm.RoleSystem:
			role = "system"
		case llm.RoleUser:
			role = "user"
		case llm.RoleAssistant:
			role = "model"
		case llm.RoleFunction, llm.RoleTool:
			role = "function"
		default:
			role = string(msg.Role)
		}

		content := Content{
			Role:  role,
			Parts: []Part{},
		}

		// Handle assistant tool calls and tool results before regular content
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				content.Parts = append(content.Parts, Part{
					FunctionCall: &FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
			}
			req.Contents = append(req.Contents, content)
			continue
		}
		if msg.Role == llm.RoleTool && msg.ToolCallId != "" {
			if len(msg.Items) > 0 {
				for _, item := range msg.Items {
					text := item.Data
					content.Parts = append(content.Parts, Part{
						FunctionResponse: &FunctionResponse{
							Name:     msg.Name,
							Response: text,
						},
					})
				}
			} else if msg.Content != "" {
				content.Parts = append(content.Parts, Part{
					FunctionResponse: &FunctionResponse{
						Name:     msg.Name,
						Response: msg.Content,
					},
				})
			}
			req.Contents = append(req.Contents, content)
			continue
		}

		// Handle content based on priority: Items > ContentItems > Result
		if len(msg.Items) > 0 {
			// Convert Items to Gemini format
			for _, item := range msg.Items {
				switch item.Type {
				case llm.ContentTypeText:
					// Use Data field first, fall back to Text field
					text := item.Data
					if text == "" {
						text = item.Text
					}
					content.Parts = append(content.Parts, Part{
						Text: text,
					})
				case llm.ContentTypeImage, llm.ContentTypeImageURL:
					// Handle image content
					if item.Source == llm.SourceURL && item.Data != "" {

						mimeType := item.MimeType
						ext := path.Ext(url.Path(item.Data))
						if mimeType == "" {
							mimeType = mime.TypeByExtension(ext)
							if mimeType == "" {
								mimeType = "image/jpeg"
							}
						}

						// Check if the URL is a file URI (starts with file://)
						if strings.Contains(item.Data, "://") {

							schema := url.Scheme(item.Data, file.Scheme)
							switch schema {
							case file.Scheme:
								imagePart, err := downloadImagePart(ctx, fs, item, mimeType)
								if err != nil {
									return nil, err
								}
								content.Parts = append(content.Parts, *imagePart)
							case http.Scheme, http.SecureScheme:
								imagePart, err := downloadImagePart(ctx, fs, item, mimeType)
								if err != nil {
									return nil, err
								}
								content.Parts = append(content.Parts, *imagePart)
							case "gs":
								content.Parts = append(content.Parts, Part{
									FileData: &FileData{
										MimeType: mimeType, // Assuming JPEG, adjust as needed
										FileURI:  item.Data,
									},
								})
							}

						} else {
							content.Parts = append(content.Parts, Part{
								InlineData: &InlineData{
									MimeType: mimeType, // Assuming JPEG, adjust as needed
									Data:     item.Data,
								},
							})
						}
					}
				case llm.ContentTypeVideo:
					// Handle video content
					if item.Source == llm.SourceURL && item.Data != "" {
						// Check if video metadata is provided
						var videoMetadata *VideoMetadata
						if item.Metadata != nil {
							startSeconds, startSecondsOk := item.Metadata["startSeconds"].(int)
							startNanos, startNanosOk := item.Metadata["startNanos"].(int)
							endSeconds, endSecondsOk := item.Metadata["endSeconds"].(int)
							endNanos, endNanosOk := item.Metadata["endNanos"].(int)

							if startSecondsOk || startNanosOk || endSecondsOk || endNanosOk {
								videoMetadata = &VideoMetadata{}

								if startSecondsOk || startNanosOk {
									videoMetadata.StartOffset = &Offset{
										Seconds: startSeconds,
										Nanos:   startNanos,
									}
								}

								if endSecondsOk || endNanosOk {
									videoMetadata.EndOffset = &Offset{
										Seconds: endSeconds,
										Nanos:   endNanos,
									}
								}
							}
						}

						// Check if the URL is a file URI (starts with file://)
						if len(item.Data) > 7 && item.Data[:7] == "file://" {
							part := Part{
								FileData: &FileData{
									MimeType: "video/mp4", // Assuming MP4, adjust as needed
									FileURI:  item.Data,
								},
							}

							if videoMetadata != nil {
								part.VideoMetadata = videoMetadata
							}

							content.Parts = append(content.Parts, part)
						} else {
							part := Part{
								InlineData: &InlineData{
									MimeType: "video/mp4", // Assuming MP4, adjust as needed
									Data:     item.Data,
								},
							}

							if videoMetadata != nil {
								part.VideoMetadata = videoMetadata
							}

							content.Parts = append(content.Parts, part)
						}
					}
				}
			}
		} else if len(msg.ContentItems) > 0 {
			// Legacy: Convert ContentItems to Gemini format
			for _, item := range msg.ContentItems {
				switch item.Type {
				case llm.ContentTypeText:
					// Use Data field first, fall back to Text field
					text := item.Data
					if text == "" {
						text = item.Text
					}
					content.Parts = append(content.Parts, Part{
						Text: text,
					})
				case llm.ContentTypeImage, llm.ContentTypeImageURL:
					// Handle image content
					if item.Source == llm.SourceURL && item.Data != "" {
						// Check if the URL is a file URI (starts with file://)
						if len(item.Data) > 7 && item.Data[:7] == "file://" {
							content.Parts = append(content.Parts, Part{
								FileData: &FileData{
									MimeType: "image/jpeg", // Assuming JPEG, adjust as needed
									FileURI:  item.Data,
								},
							})
						} else {
							content.Parts = append(content.Parts, Part{
								InlineData: &InlineData{
									MimeType: "image/jpeg", // Assuming JPEG, adjust as needed
									Data:     item.Data,
								},
							})
						}
					}
				case llm.ContentTypeVideo:
					// Handle video content
					if item.Source == llm.SourceURL && item.Data != "" {
						// Check if video metadata is provided
						var videoMetadata *VideoMetadata
						if item.Metadata != nil {
							startSeconds, startSecondsOk := item.Metadata["startSeconds"].(int)
							startNanos, startNanosOk := item.Metadata["startNanos"].(int)
							endSeconds, endSecondsOk := item.Metadata["endSeconds"].(int)
							endNanos, endNanosOk := item.Metadata["endNanos"].(int)

							if startSecondsOk || startNanosOk || endSecondsOk || endNanosOk {
								videoMetadata = &VideoMetadata{}

								if startSecondsOk || startNanosOk {
									videoMetadata.StartOffset = &Offset{
										Seconds: startSeconds,
										Nanos:   startNanos,
									}
								}

								if endSecondsOk || endNanosOk {
									videoMetadata.EndOffset = &Offset{
										Seconds: endSeconds,
										Nanos:   endNanos,
									}
								}
							}
						}

						// Check if the URL is a file URI (starts with file://)
						if len(item.Data) > 7 && item.Data[:7] == "file://" {
							part := Part{
								FileData: &FileData{
									MimeType: "video/mp4", // Assuming MP4, adjust as needed
									FileURI:  item.Data,
								},
							}

							if videoMetadata != nil {
								part.VideoMetadata = videoMetadata
							}

							content.Parts = append(content.Parts, part)
						} else {
							part := Part{
								InlineData: &InlineData{
									MimeType: "video/mp4", // Assuming MP4, adjust as needed
									Data:     item.Data,
								},
							}

							if videoMetadata != nil {
								part.VideoMetadata = videoMetadata
							}

							content.Parts = append(content.Parts, part)
						}
					}
				}
			}
		} else if msg.Content != "" {
			// Use simple string content for backward compatibility
			content.Parts = append(content.Parts, Part{
				Text: msg.Content,
			})
		}

		// Convert function call if present
		if msg.FunctionCall != nil {
			content.Parts = append(content.Parts, Part{
				FunctionCall: &FunctionCall{
					Name:      msg.FunctionCall.Name,
					Arguments: msg.FunctionCall.Arguments,
				},
			})
		}

		if role == string(llm.RoleSystem) {
			req.SystemInstruction = &SystemInstruction{
				Role:  role,
				Parts: content.Parts,
			}
			continue
		}

		// Add content to request
		req.Contents = append(req.Contents, content)
	}

	return req, nil
}

func downloadImagePart(ctx context.Context, fs afs.Service, item llm.ContentItem, mimeType string) (*Part, error) {
	imageBytes, err := fs.DownloadWithURL(ctx, item.Data)
	if err != nil {
		return nil, err
	}
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)
	imagePart := &Part{
		InlineData: &InlineData{
			MimeType: mimeType, // Assuming JPEG, adjust as needed
			Data:     base64Image,
		},
	}
	return imagePart, nil
}

// ToLLMSResponse converts a Response to an llm.ChatResponse
func ToLLMSResponse(resp *Response) *llm.GenerateResponse {
	// Create the LLMS response
	llmsResp := &llm.GenerateResponse{
		Choices: make([]llm.Choice, 0, len(resp.Candidates)),
	}

	// Convert candidates to choices
	for i, candidate := range resp.Candidates {
		llmsChoice := llm.Choice{
			Index:        i,
			FinishReason: candidate.FinishReason,
		}

		// Create the message with basic fields
		message := llm.Message{
			Role: llm.RoleAssistant, // Gemini uses "model" for assistant
		}

		// Handle content parts
		if len(candidate.Content.Parts) > 0 {
			// Extract text content
			var textContent string
			message.Items = make([]llm.ContentItem, 0)
			message.ContentItems = make([]llm.ContentItem, 0)

			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					// Append to full text content
					if textContent != "" {
						textContent += "\n"
					}
					textContent += part.Text

					// Create metadata for additional fields
					metadata := make(map[string]interface{})

					// Add citation metadata if available
					if candidate.CitationMetadata != nil && len(candidate.CitationMetadata.Citations) > 0 {
						metadata["citations"] = candidate.CitationMetadata.Citations
					}

					// Add logprobs if available
					if candidate.LogprobsResult != nil {
						metadata["logprobs"] = candidate.LogprobsResult
					}

					// Add avgLogprobs if available
					if candidate.AvgLogprobs != 0 {
						metadata["avgLogprobs"] = candidate.AvgLogprobs
					}

					// Add model version if available
					if resp.ModelVersion != "" {
						metadata["modelVersion"] = resp.ModelVersion
					}

					// Add as content item
					contentItem := llm.ContentItem{
						Type:     llm.ContentTypeText,
						Source:   llm.SourceRaw,
						Data:     part.Text,
						Text:     part.Text,
						Metadata: metadata,
					}
					message.Items = append(message.Items, contentItem)
					message.ContentItems = append(message.ContentItems, contentItem)
				} else if part.FunctionCall != nil {
					// Handle function call
					message.FunctionCall = &llm.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: part.FunctionCall.Arguments,
					}
				}
			}

			// Set the full text content
			message.Content = textContent
		}

		llmsChoice.Message = message
		llmsResp.Choices = append(llmsResp.Choices, llmsChoice)
	}

	// Convert usage if available
	if resp.UsageMetadata != nil {
		llmsResp.Usage = &llm.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	return llmsResp
}

package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"path"
	"reflect"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/afs/http"
	"github.com/viant/afs/url"
	"github.com/viant/agently/genai/llm"
)

// ToRequest converts an llm.ChatRequest to a Gemini Request
func ToRequest(ctx context.Context, request *llm.GenerateRequest) (*Request, error) {
	if request == nil {
		return nil, fmt.Errorf("nil generate request")
	}
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

		// Final sweep: ensure all function declarations are sanitized (handles any
		// future mutations above).
		for ti := range req.Tools {
			for fi := range req.Tools[ti].FunctionDeclarations {
				fd := &req.Tools[ti].FunctionDeclarations[fi]
				if fd.Parameters != nil {
					fd.Parameters = sanitizeSchema(fd.Parameters).(map[string]interface{})
				}
			}
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

		// Thinking budget (Gemini 2.5 specific)
		if thinking := request.Options.Thinking; thinking != nil {
			req.GenerationConfig.ThinkingConfig = &ThinkingConfig{ThinkingBudget: thinking.BudgetTokens}
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

		// Prepare slice for allowed function names across all declared tools
		var funcNames []string

		// Convert tools if provided
		if len(request.Options.Tools) > 0 {
			req.Tools = make([]Tool, 1)
			req.Tools[0].FunctionDeclarations = make([]FunctionDeclaration, len(request.Options.Tools))

			for i, tool := range request.Options.Tools {
				// Always assign a fully sanitised parameters map; this guarantees
				// that unsupported keys (e.g. additionalProperties) are removed
				// at every nesting level.
				var params map[string]interface{}
				if tool.Definition.Parameters != nil {
					params = sanitizeSchema(tool.Definition.Parameters).(map[string]interface{})
				}
				req.Tools[0].FunctionDeclarations[i] = FunctionDeclaration{
					Name:        tool.Definition.Name,
					Description: tool.Definition.Description,
					Parameters:  params,
				}
				// Capture function name for allowed list
				funcNames = append(funcNames, tool.Definition.Name)
			}

		}

		// --------------------------------------------------------------
		// Attach toolConfig with mode + allowed function names.
		// --------------------------------------------------------------
		// Map ToolChoice to Gemini mode (default AUTO when unspecified)
		var mode string
		switch request.Options.ToolChoice.Type {
		case "":
			mode = "AUTO"
		case "auto":
			mode = "AUTO"
		case "none":
			mode = "NONE"
		case "function":
			mode = "ANY"
		}

		if len(funcNames) > 0 {
			if req.ToolConfig == nil {
				req.ToolConfig = &ToolConfig{FunctionCallingConfig: &FunctionCallingConfig{}}
			} else if req.ToolConfig.FunctionCallingConfig == nil {
				req.ToolConfig.FunctionCallingConfig = &FunctionCallingConfig{}
			}
			// Preserve any previously set mode unless new mode provided.
			if mode != "" {
				req.ToolConfig.FunctionCallingConfig.Mode = mode
			}
			// Populate allowed_function_names only when mode == "ANY".
			if mode == "ANY" && len(funcNames) > 0 {
				req.ToolConfig.FunctionCallingConfig.AllowedFunctionNames = funcNames
			}
		}
	}

	wasSystemMsg := false
	if strings.TrimSpace(request.Instructions) != "" {
		req.SystemInstruction = &SystemInstruction{
			Role:  "system",
			Parts: []Part{{Text: strings.TrimSpace(request.Instructions)}},
		}
		wasSystemMsg = true
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

		// Special handling for system messages: send via top-level systemInstruction
		if msg.Role == llm.RoleSystem {
			if !wasSystemMsg {
				req.SystemInstruction = &SystemInstruction{
					Role:  "system",
					Parts: []Part{},
				}
				wasSystemMsg = true
			}

			// If caller provided explicit parts, use them, otherwise wrap msg.Content
			if len(msg.Items) == 0 {
				req.SystemInstruction.Parts = append(req.SystemInstruction.Parts, Part{Text: llm.MessageText(msg)})
			} else {
				for _, item := range msg.Items {
					if item.Type == llm.ContentTypeText {
						text := item.Data
						if text == "" {
							text = item.Text
						}
						req.SystemInstruction.Parts = append(req.SystemInstruction.Parts, Part{Text: text})
					}
				}
			}
			continue
		}

		content := Content{
			Role:  role,
			Parts: []Part{},
		}

		// Handle assistant tool calls and tool results before regular content
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				ts := strings.TrimSpace(tc.ID)
				if strings.HasPrefix(ts, EMPTY_THOUGHT_SIGNATURE) {
					ts = ""
				}
				part := Part{
					ThoughtSignature: ts,
					FunctionCall: &FunctionCall{
						Name: tc.Name,
						Args: tc.Arguments,
					},
				}

				content.Parts = append(content.Parts, part)
			}
			req.Contents = append(req.Contents, content)
			continue
		}
		if msg.Role == llm.RoleTool && msg.ToolCallId != "" {
			// As per Gemini doc, functionResponse must have role "user".
			content.Role = "user"
			// Tool results may carry binary attachments (e.g. images). Gemini requires
			// functionResponse payloads to be JSON objects; attachments must be sent
			// as separate parts (inlineData/fileData), not embedded in the response object.
			toolResponse := strings.TrimSpace(msg.Content)
			if toolResponse == "" && len(msg.Items) > 0 {
				for _, item := range msg.Items {
					if item.Type != llm.ContentTypeText {
						continue
					}
					text := strings.TrimSpace(item.Data)
					if text == "" {
						text = strings.TrimSpace(item.Text)
					}
					if text != "" {
						toolResponse = text
						break
					}
				}
			}

			part := Part{
				FunctionResponse: &FunctionResponse{
					Name:     msg.Name,
					Response: parseJSONOrString(toolResponse),
				},
			}

			//toolResponse
			content.Parts = append(content.Parts, part)
			// Append binary image parts for vision.
			for _, item := range msg.Items {
				switch item.Type {
				case llm.ContentTypeBinary:
					if item.Data == "" {
						continue
					}
					mimeType := strings.TrimSpace(item.MimeType)
					if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
						continue
					}
					content.Parts = append(content.Parts, Part{
						InlineData: &InlineData{
							MimeType: mimeType,
							Data:     item.Data,
						},
					})
				case llm.ContentTypeImage, llm.ContentTypeImageURL:
					if item.Source != llm.SourceURL || strings.TrimSpace(item.Data) == "" {
						continue
					}
					mimeType := strings.TrimSpace(item.MimeType)
					if mimeType == "" {
						ext := path.Ext(url.Path(item.Data))
						mimeType = mime.TypeByExtension(ext)
						if mimeType == "" {
							mimeType = "image/jpeg"
						}
					}
					imagePart, err := downloadImagePart(ctx, fs, item, mimeType)
					if err != nil {
						return nil, err
					}
					content.Parts = append(content.Parts, *imagePart)
				}
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
					if msg.Role == llm.RoleUser && strings.TrimSpace(msg.Name) != "" {
						text = msg.Name + ":" + text
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
				case llm.ContentTypeBinary:
					// Generic inline binary using provided MIME type
					if item.Data != "" {
						content.Parts = append(content.Parts, Part{
							InlineData: &InlineData{
								MimeType: item.MimeType,
								Data:     item.Data,
							},
						})
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
				case llm.ContentTypeBinary:
					// Generic inline binary using provided MIME type
					if item.Data != "" {
						content.Parts = append(content.Parts, Part{
							InlineData: &InlineData{
								MimeType: item.MimeType,
								Data:     item.Data,
							},
						})
					}
				}
			}
		} else if msg.Content != "" {
			// Use simple string content for backward compatibility
			text := msg.Content
			if msg.Role == llm.RoleUser && strings.TrimSpace(msg.Name) != "" {
				text = msg.Name + ":" + text
			}
			content.Parts = append(content.Parts, Part{
				Text: text,
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

		// Add content to request
		req.Contents = append(req.Contents, content)
	}

	req.Contents = normalizeToolCallResponsePairSubSlices(req.Contents)

	// Gemini v1beta expects the conversation to start with a USER turn and to
	// alternate roles.  If for any reason the accumulated messages begin with
	// a model/function call we prepend an empty USER message to satisfy the
	// protocol (avoids 400 "function call must come after user turn").
	if len(req.Contents) > 0 {
		firstRole := req.Contents[0].Role
		if firstRole == "model" || firstRole == "function" || firstRole == "assistant" {
			// insert placeholder user message at index 0
			req.Contents = append([]Content{{Role: "user", Parts: []Part{{Text: " "}}}}, req.Contents...)
		}
	}

	return req, nil
}

type toolCallResponsePair struct {
	call Content
	resp Content
}

// normalizeToolCallResponsePairSubSlices groups consecutive (model functionCall, user functionResponse)
// pairs into sub-slices where at most one model-side content carries a non-empty thoughtSignature.
// If any model-side content in a sub-slice contains an empty thoughtSignature, that sub-slice is
// collapsed into a single aggregated (model, user) pair. Within that aggregation, pairs with any
// non-empty thoughtSignature are emitted first (stable), followed by those with none.
func normalizeToolCallResponsePairSubSlices(contents []Content) []Content {
	if len(contents) < 2 {
		return contents
	}

	isToolCall := func(c *Content) bool {
		if c == nil || strings.TrimSpace(c.Role) != "model" {
			return false
		}
		for i := range c.Parts {
			if c.Parts[i].FunctionCall != nil {
				return true
			}
		}
		return false
	}
	isToolResponse := func(c *Content) bool {
		if c == nil || strings.TrimSpace(c.Role) != "user" {
			return false
		}
		for i := range c.Parts {
			if c.Parts[i].FunctionResponse != nil {
				return true
			}
		}
		return false
	}
	hasNonEmptyThoughtSignature := func(call *Content) bool {
		if call == nil {
			return false
		}
		for i := range call.Parts {
			p := &call.Parts[i]
			if p.FunctionCall == nil {
				continue
			}
			if strings.TrimSpace(p.ThoughtSignature) != "" {
				return true
			}
		}
		return false
	}
	hasEmptyThoughtSignature := func(call *Content) bool {
		if call == nil {
			return false
		}
		for i := range call.Parts {
			p := &call.Parts[i]
			if p.FunctionCall == nil {
				continue
			}
			if strings.TrimSpace(p.ThoughtSignature) == "" {
				return true
			}
		}
		return false
	}

	out := make([]Content, 0, len(contents))
	for i := 0; i < len(contents); {
		// 1) Extract subslice of consecutive (A,B) pairs.
		if i+1 < len(contents) && isToolCall(&contents[i]) && isToolResponse(&contents[i+1]) {
			pairs := make([]toolCallResponsePair, 0, 4)
			// A subslice is a run of consecutive (call,response) pairs where at most
			// one call content has a non-empty thoughtSignature. The subslice ends
			// before a pair whose call would introduce a second non-empty signature.
			seenNonEmpty := false
			for {
				pairs = append(pairs, toolCallResponsePair{call: contents[i], resp: contents[i+1]})
				if !seenNonEmpty && hasNonEmptyThoughtSignature(&contents[i]) {
					seenNonEmpty = true
				}
				i += 2
				if i+1 >= len(contents) || !isToolCall(&contents[i]) || !isToolResponse(&contents[i+1]) {
					break
				}
				// If we've already seen a non-empty thoughtSignature, stop before the
				// next pair when it would introduce a second non-empty signature.
				if seenNonEmpty && hasNonEmptyThoughtSignature(&contents[i]) {
					break
				}
			}

			// 2) If all model sides have non-empty thoughtSignature(s), do nothing.
			needsAggregation := false
			for pi := range pairs {
				if hasEmptyThoughtSignature(&pairs[pi].call) {
					needsAggregation = true
					break
				}
			}
			if !needsAggregation {
				for pi := range pairs {
					out = append(out, pairs[pi].call, pairs[pi].resp)
				}
				continue
			}

			// 3) Aggregate: stable partition by "has any non-empty thoughtSignature".
			aggCall := Content{Role: "model", Parts: make([]Part, 0)}
			aggResp := Content{Role: "user", Parts: make([]Part, 0)}

			for pi := range pairs {
				if !hasNonEmptyThoughtSignature(&pairs[pi].call) {
					continue
				}
				aggCall.Parts = append(aggCall.Parts, pairs[pi].call.Parts...)
				aggResp.Parts = append(aggResp.Parts, pairs[pi].resp.Parts...)
			}
			for pi := range pairs {
				if hasNonEmptyThoughtSignature(&pairs[pi].call) {
					continue
				}
				aggCall.Parts = append(aggCall.Parts, pairs[pi].call.Parts...)
				aggResp.Parts = append(aggResp.Parts, pairs[pi].resp.Parts...)
			}

			out = append(out, aggCall, aggResp)
			continue
		}

		out = append(out, contents[i])
		i++
	}

	return out
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

// sanitizeSchema removes fields that are not accepted by Gemini v1beta
// (e.g., additionalProperties) and recurses into nested objects.

func sanitizeSchema(v interface{}) interface{} {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		cleaned := make(map[string]interface{}, rv.Len())
		for _, key := range rv.MapKeys() {

			kStr := fmt.Sprintf("%v", key.Interface())
			if kStr == "additionalProperties" || strings.HasPrefix(kStr, "x-") {
				continue
			}
			cleaned[kStr] = sanitizeSchema(rv.MapIndex(key).Interface())
		}
		return cleaned
	case reflect.Slice, reflect.Array:
		length := rv.Len()
		arr := make([]interface{}, length)
		for i := 0; i < length; i++ {
			arr[i] = sanitizeSchema(rv.Index(i).Interface())
		}
		return arr
	default:
		return v
	}
}

// parseJSONOrString normalizes tool/function responses to an object as required by Gemini.
// - If s is a valid JSON object, it is returned as-is (map[string]interface{}).
// - If s is valid JSON but not an object (array/number/bool/string/null), it is wrapped:
//   - array -> {"data": <array>}
//   - scalar -> {"value": <scalar>}
//
// - If s is not valid JSON, we wrap it as an error payload: {"error": {"message": s}}
// This guarantees Gemini receives a protobuf Struct-compatible value (JSON object),
// avoiding INVALID_ARGUMENT when a plain string was provided.
func parseJSONOrString(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		switch tv := v.(type) {
		case map[string]interface{}:
			// Already an object – OK
			return tv
		case []interface{}:
			return map[string]interface{}{"data": tv}
		default:
			return map[string]interface{}{"value": tv}
		}
	}
	// Not JSON – treat as error string to preserve failure context while returning an object
	return map[string]interface{}{
		"error": map[string]interface{}{
			"message": s,
		},
	}
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
					// Convert Gemini functionCall into llm.ToolCall (preferred) and also
					// keep legacy FunctionCall for backward compatibility.
					var argsMap map[string]interface{}
					if part.FunctionCall.Args != nil {
						if m, ok := part.FunctionCall.Args.(map[string]interface{}); ok {
							argsMap = m
						}
					} else if part.FunctionCall.Arguments != "" {
						_ = json.Unmarshal([]byte(part.FunctionCall.Arguments), &argsMap)
					}

					message.ToolCalls = append(message.ToolCalls, llm.ToolCall{
						Name:      part.FunctionCall.Name,
						Arguments: argsMap,
					})

					// Keep legacy field for clients relying on it
					if part.FunctionCall.Arguments != "" {
						message.FunctionCall = &llm.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: part.FunctionCall.Arguments,
						}
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

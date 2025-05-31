package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/fluxor/model/types"
	"github.com/viant/velty"
)

type GenerateInput struct {
	Model        string
	SystemPrompt string
	Prompt       string
	Attachment   []*Attachment
	Message      []llms.MessageContent
	Tools        []llm.Tool
    // Options allows callers to specify advanced llm.Options (temperature,
    // top-p, etc.).  If nil a minimal options struct will be created that only
    // carries Tools.
    Options      *llm.Options
	Template     string
	Bind         map[string]interface{}
}

// GenerateOutput represents output from extraction
type GenerateOutput struct {
	Response *llm.GenerateResponse
	Content  string
}

func (i *GenerateInput) Init(ctx context.Context) {
	if len(i.Message) == 0 {
		i.Message = []llms.MessageContent{}
		if i.SystemPrompt != "" {
			i.Message = append(i.Message, llms.TextParts(llms.ChatMessageTypeSystem, i.SystemPrompt))
		}
		if i.Prompt != "" {
			i.Message = append(i.Message, llms.TextParts(llms.ChatMessageTypeHuman, i.Prompt))
		}

		for _, attachment := range i.Attachment {
			parts := []llms.ContentPart{
				llms.BinaryPart(attachment.MIMEType(), attachment.Data),
			}
			if attachment.Prompt != "" {
				parts = append(parts, llms.TextPart(attachment.Prompt))
			}

			i.Message = append(i.Message, llms.MessageContent{
				Role:  llms.ChatMessageTypeHuman,
				Parts: parts,
			})
		}
	}
}

func (i *GenerateInput) Validate(ctx context.Context) error {
	if i.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(i.Message) == 0 {
		return fmt.Errorf("content is required")
	}
	return nil
}

// generate processes LLM responses to generate structured data
func (s *Service) generate(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*GenerateInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*GenerateOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	return s.Generate(ctx, input, output)
}

func (s *Service) Generate(ctx context.Context, input *GenerateInput, output *GenerateOutput) error {
	if input.Prompt == "" && input.Template != "" {
		expanded, err := s.expandTemplate(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to expand template: %w", err)
		}
		input.Prompt = expanded
	}
	input.Init(ctx)
	if err := input.Validate(ctx); err != nil {
		return err
	}

	model, err := s.llmFinder.Find(ctx, input.Model)
	if err != nil {
		return fmt.Errorf("failed to find model: %w", err)
	}

	// Convert langchaingo messages to llm.Message
	messages := make([]llm.Message, 0, len(input.Message))
	for _, msg := range input.Message {
		items := make([]llm.ContentItem, 0)
		// Since we can't directly access the langchaingo types, we'll use a type switch
		// to determine the content type and extract the data
		for i := range msg.Parts {
			// Create a content item based on the part type
			var item llm.ContentItem

			// Determine the content type and extract the data
			// This is a best-effort approach without knowing the exact structure
			switch {
			case strings.Contains(fmt.Sprintf("%T", msg.Parts[i]), "Text"):
				item = llm.ContentItem{
					Type:   llm.ContentTypeText,
					Source: llm.SourceRaw,
					Data:   fmt.Sprintf("%v", msg.Parts[i]),
				}
			case strings.Contains(fmt.Sprintf("%T", msg.Parts[i]), "Image"):
				item = llm.ContentItem{
					Type:   llm.ContentTypeImage,
					Source: llm.SourceURL,
					Data:   fmt.Sprintf("%v", msg.Parts[i]),
				}
			case strings.Contains(fmt.Sprintf("%T", msg.Parts[i]), "Binary"):
				item = llm.ContentItem{
					Type:   llm.ContentTypeBinary,
					Source: llm.SourceRaw,
					Data:   fmt.Sprintf("%v", msg.Parts[i]),
				}
			default:
				// Skip unknown content types
				continue
			}

			items = append(items, item)
		}

		// Determine the role based on the msg.Role string representation
		var role llm.MessageRole
		roleStr := fmt.Sprintf("%v", msg.Role)
		switch {
		case strings.Contains(roleStr, "Human"):
			role = llm.RoleUser
		case strings.Contains(roleStr, "AI"):
			role = llm.RoleAssistant
		case strings.Contains(roleStr, "System"):
			role = llm.RoleSystem
		case strings.Contains(roleStr, "Tool"):
			role = llm.RoleTool
		default:
			// Default to user role for unknown types
			role = llm.RoleUser
		}

		messages = append(messages, llm.Message{
			Role:  role,
			Items: items,
		})
	}

	// Build options – start with caller-supplied structure when present so we
	// preserve Temperature, TopP, etc.
	var opts *llm.Options
	if input.Options != nil {
		clone := *input.Options // shallow copy is enough (maps are reference but we don't mutate)
		opts = &clone
	} else {
		opts = &llm.Options{}
	}

	// Ensure tools list is kept
	if len(opts.Tools) == 0 && len(input.Tools) > 0 {
		opts.Tools = input.Tools
	}

	request := &llm.GenerateRequest{
		Messages: messages,
		Options:  opts,
	}

	response, err := model.Generate(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}
	output.Response = response

	// Usage aggregation is now handled by provider-level UsageListener attached
	// in the model finder. Avoid double-counting here.
	var builder strings.Builder
	for _, choice := range response.Choices {
		if txt := strings.TrimSpace(choice.Message.Content); txt != "" {
			builder.WriteString(txt)
			continue // prefer Content when provided, avoid double printing
		}
		for _, item := range choice.Message.Items {
			if item.Type != llm.ContentTypeText {
				continue
			}
			if item.Data != "" {
				builder.WriteString(item.Data)
			} else if item.Text != "" {
				builder.WriteString(item.Text)
			}
		}
	}
	output.Content = strings.TrimSpace(builder.String())
	return nil
}

func EnsureJSONResponse(ctx context.Context, text string, target interface{}) error {
	if strings.HasPrefix(text, "```") {
		if idx := strings.Index(text, "\n"); idx != -1 {
			text = text[idx+1:]
		}
		if end := strings.LastIndex(text, "```"); end != -1 {
			text = text[:end]
		}
		text = strings.TrimSpace(text)
	}

	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("failed to unmarshal LLM text into %T: %w", target, err)
	}
	return nil
}

func (s *Service) expandTemplate(ctx context.Context, input *GenerateInput) (string, error) {
	planner := velty.New()
	err := planner.DefineVariable("Prompt", input.Prompt)
	if err != nil {
		return "", err
	}
	// Define variables once during compilation
	for k, v := range input.Bind {
		if err := planner.DefineVariable(k, v); err != nil {
			return "", err
		}
	}
	exec, newState, err := planner.Compile([]byte(input.Template))
	if err != nil {
		return "", err
	}
	state := newState()
	state.SetValue("Prompt", input.Prompt)
	// Define variables once during compilation
	for k, v := range input.Bind {
		if err := state.SetValue(k, v); err != nil {
			return "", err
		}
	}
	// No need to set values again, they were already defined during compilation
	if err = exec.Exec(state); err != nil {
		return "", err
	}
	output := string(state.Buffer.Bytes())
	return output, nil
}

type Attachment struct {
	Name   string
	Mime   string
	Prompt string
	Data   []byte
}

func (a *Attachment) MIMEType() string {
	if a.Mime != "" {
		return a.Mime
	}
	// Handle empty Name case
	if a.Name == "" {
		return "application/octet-stream"
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(a.Name), "."))
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "pdf":
		return "application/pdf"
	case "txt":
		return "text/plain"
	case "md":
		return "text/markdown"
	case "csv":
		return "text/csv"
	case "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "html":
		return "text/html"
	case "yaml", "yml":
		return "application/x-yaml"
	case "zip":
		return "application/zip"
	case "tar":
		return "application/x-tar"
	case "mp3":
		return "audio/mpeg"
	case "mp4":
		return "video/mp4"
	}
	return "application/octet-stream"
}

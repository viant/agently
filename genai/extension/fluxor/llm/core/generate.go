package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
	fluxortypes "github.com/viant/fluxor/model/types"
	"github.com/viant/velty"

	elog "github.com/viant/agently/internal/log"
)

type GenerateInput struct {
	Model        string
	Preferences  *llm.ModelPreferences // optional model preferences
	SystemPrompt string
	Prompt       string
	Attachment   []*Attachment
	Message      []llm.Message
	Tools        []llm.Tool

	// Options allows callers to specify advanced llm.Options (temperature,
	// top-p, etc.).  If nil a minimal options struct will be created that only
	// carries Tools.
	Options  *llm.Options
	Template string
	Bind     map[string]interface{}
}

// GenerateOutput represents output from extraction
type GenerateOutput struct {
	Response *llm.GenerateResponse
	Content  string
}

func (i *GenerateInput) Init(ctx context.Context) {
	if i.SystemPrompt != "" {
		i.Message = append(i.Message, llm.NewSystemMessage(i.SystemPrompt))
	}
	if i.Prompt != "" {
		i.Message = append(i.Message, llm.NewUserMessage(i.Prompt))
	}
	for _, attachment := range i.Attachment {
		i.Message = append(i.Message,
			llm.NewUserMessageWithBinary(attachment.Data, attachment.MIMEType(), attachment.Prompt))
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
		return fluxortypes.NewInvalidInputError(in)
	}
	output, ok := out.(*GenerateOutput)
	if !ok {
		return fluxortypes.NewInvalidOutputError(out)
	}

	return s.Generate(ctx, input, output)
}

func (s *Service) Generate(ctx context.Context, input *GenerateInput, output *GenerateOutput) error {
	request, model, err := s.prepareGenerateRequest(ctx, input)
	if err != nil {
		return err
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

	// --------------------------------------------------------------
	// Optional logging
	// --------------------------------------------------------------
	elog.Publish(elog.Event{Time: time.Now(), EventType: elog.LLMInput, Payload: request})
	elog.Publish(elog.Event{Time: time.Now(), EventType: elog.LLMOutput, Payload: response})

	if s.logWriter != nil {
		req, _ := json.Marshal(request)
		s.logWriter.Write(append(req, '\n'))
		resp, _ := json.Marshal(response)
		s.logWriter.Write(append(resp, '\n'))
	}
	return nil
}

// EnsureJSONResponse extracts and unmarshals valid JSON content from a given string into the target interface.
// It trims potential code block markers and identifies the JSON object or array to parse.
// Returns an error if no valid JSON is found or if unmarshalling fails.
func EnsureJSONResponse(ctx context.Context, text string, target interface{}) error {
	// Strip code block markers
	if strings.HasPrefix(text, "```") {
		if idx := strings.Index(text, "\n"); idx != -1 {
			text = text[idx+1:]
		}
		if end := strings.LastIndex(text, "```"); end != -1 {
			text = text[:end]
		}
		text = strings.TrimSpace(text)
	}

	// Extract likely JSON content
	objectStart := strings.Index(text, "{")
	objectEnd := strings.LastIndex(text, "}")
	arrayStart := strings.Index(text, "[")
	arrayEnd := strings.LastIndex(text, "]")

	switch {
	case objectStart != -1 && objectEnd != -1 && (arrayStart == -1 || objectStart < arrayStart):
		text = text[objectStart : objectEnd+1]
	case arrayStart != -1 && arrayEnd != -1:
		text = text[arrayStart : arrayEnd+1]
	default:
		//regular response
		return nil
	}
	// Attempt to parse JSON
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("failed to unmarshal LLM text into %T: %w\nRaw text: %s", target, err, text)
	}
	return nil
}

// prepareGenerateRequest prepares a GenerateRequest and resolves the model based
// on preferences or defaults. It expands templates, validates input, and clones options.
func (s *Service) prepareGenerateRequest(ctx context.Context, input *GenerateInput) (*llm.GenerateRequest, llm.Model, error) {
	if input.Prompt == "" && input.Template != "" {
		expanded, err := s.expandTemplate(ctx, input)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to expand template: %w", err)
		}
		input.Prompt = expanded
	}

	if input.Model == "" {
		if input.Preferences != nil {
			if m := s.modelMatcher.Best(input.Preferences); m != "" {
				input.Model = m
			}
		}
		if input.Model == "" {
			input.Model = s.defaultModel
		}
	}

	input.Init(ctx)
	if err := input.Validate(ctx); err != nil {
		return nil, nil, err
	}

	model, err := s.llmFinder.Find(ctx, input.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find model: %w", err)
	}

	messages := input.Message
	var opts *llm.Options
	if input.Options != nil {
		clone := *input.Options
		opts = &clone
	} else {
		opts = &llm.Options{}
	}

	if len(opts.Tools) == 0 && len(input.Tools) > 0 {
		opts.Tools = input.Tools
	}

	request := &llm.GenerateRequest{
		Messages: messages,
		Options:  opts,
	}

	return request, model, nil
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

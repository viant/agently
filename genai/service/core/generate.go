package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	modelcallctx "github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/prompt"
	fluxortypes "github.com/viant/fluxor/model/types"
)

type GenerateInput struct {
	llm.ModelSelection
	SystemPrompt *prompt.Prompt

	Prompt  *prompt.Prompt
	Binding *prompt.Binding
	Message []llm.Message
}

// GenerateOutput represents output from extraction
type GenerateOutput struct {
	Response  *llm.GenerateResponse
	Content   string
	MessageID string
}

func (i *GenerateInput) MatchModelIfNeeded(matcher llm.Matcher) {
	if i.Model != "" || i.Preferences == nil {
		return
	}
	if m := matcher.Best(i.Preferences); m != "" {
		i.Model = m
	}
}

func (i *GenerateInput) Init(ctx context.Context) error {

	if i.SystemPrompt != nil {
		if err := i.SystemPrompt.Init(ctx); err != nil {
			return err
		}
		expanded, err := i.SystemPrompt.Generate(ctx, i.Binding.SystemBinding())
		if err != nil {
			return fmt.Errorf("failed to expand system prompt: %w", err)
		}
		i.Message = append(i.Message, llm.NewSystemMessage(expanded))
	}

	// Note: attachments are appended later in prepareGenerateRequest after
	// model capabilities are known (IsMultimodal flag).

	if i.Prompt == nil {
		i.Prompt = &prompt.Prompt{}
	}
	if err := i.Prompt.Init(ctx); err != nil {
		return err
	}
	currentPrompt, err := i.Prompt.Generate(ctx, i.Binding)
	if err != nil {
		return fmt.Errorf("failed to prompt: %w", err)
	}

	if i.Binding != nil && len(i.Binding.History.Messages) > 0 {
		messages := i.Binding.History.Messages
		for k := 0; k < len(messages); k++ {
			m := messages[k]
			sortAttachments(m.Attachment)
			for _, attachment := range m.Attachment {
				i.Message = append(i.Message,
					llm.NewMessageWithBinary(llm.MessageRole(m.Role), attachment.Data, attachment.MIMEType(), attachment.Content))
			}
			llmMessage := llm.NewTextMessage(llm.MessageRole(m.Role), m.Content)
			i.Message = append(i.Message, llmMessage)
		}
	}

	if tools := i.Binding.Tools; len(tools.Signatures) > 0 {
		for _, tool := range tools.Signatures {
			i.Options.Tools = append(i.Options.Tools, llm.Tool{Ref: "", Definition: *tool})
		}
		for _, call := range tools.Executions {
			i.Message = append(i.Message, llm.NewAssistantMessageWithToolCalls(*call))
			i.Message = append(i.Message, llm.NewToolResultMessage(*call))
		}
	}

	i.Message = append(i.Message, llm.NewUserMessage(currentPrompt))
	return nil
}

func sortAttachments(attachments []*prompt.Attachment) {
	sort.Slice(attachments, func(i, j int) bool {
		if attachments[i] == nil || attachments[j] == nil {
			return false
		}
		if strings.Compare(attachments[i].URI, attachments[j].URI) < 0 {
			return true
		}
		return false
	})
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

	ctx = modelcallctx.WithRecorderObserver(ctx, s.convClient)
	request, model, err := s.prepareGenerateRequest(ctx, input)
	if err != nil {
		return err
	}

	// Attach finish barrier to upstream ctx so recorder observer can signal completion (payload ids, usage).
	ctx, _ = modelcallctx.WithFinishBarrier(ctx)
	response, err := model.Generate(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}
	output.Response = response

	// Usage aggregation is now handled by provider-level UsageListener attached
	// in the model finder. Avoid double-counting here.
	var builder strings.Builder
	for _, choice := range response.Choices {
		if len(choice.Message.ToolCalls) > 0 {
			continue
		}
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
	// Provide the shared assistant message ID to the caller; orchestrator writes the final assistant message.
	if msgID := memory.ModelMessageIDFromContext(ctx); msgID != "" {
		output.MessageID = msgID
	}
	return nil
}

// prepareGenerateRequest prepares a GenerateRequest and resolves the model based
// on preferences or defaults. It expands templates, validates input, and clones options.
func (s *Service) prepareGenerateRequest(ctx context.Context, input *GenerateInput) (*llm.GenerateRequest, llm.Model, error) {

	input.MatchModelIfNeeded(s.modelMatcher)

	if input.Binding == nil {
		input.Binding = &prompt.Binding{}
	}

	model, err := s.llmFinder.Find(ctx, input.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find model: %w", err)
	}
	s.updateFlags(input, model)
	if err := input.Init(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to init generate input: %w", err)
	}
	if err := input.Validate(ctx); err != nil {
		return nil, nil, err
	}

	request := &llm.GenerateRequest{
		Messages: input.Message,
		Options:  input.Options,
	}
	return request, model, nil
}

func (s *Service) updateFlags(input *GenerateInput, model llm.Model) {
	input.Binding.Flags.CanUseTool = model.Implements(base.CanUseTools)
	input.Binding.Flags.CanStream = model.Implements(base.CanStream)
	input.Binding.Flags.IsMultimodal = model.Implements(base.IsMultimodal)

	// Gate parallel tool-calls option based on provider/model support.
	// If the agent config requested parallel tool calls but the model
	// doesn’t implement the capability, force-disable it to avoid
	// sending unsupported fields downstream.
	if input.Options != nil && input.Options.ParallelToolCalls {
		if !model.Implements(base.CanExecToolsInParallel) {
			input.Options.ParallelToolCalls = false
		}
	}
}

//
//func attachmentMIME(a *prompt.Attachment) string {
//	if a == nil {
//		return "application/octet-Stream"
//	}
//	if strings.TrimSpace(a.Mime) != "" {
//		return a.Mime
//	}
//	name := strings.TrimSpace(a.Name)
//	if name == "" {
//		return "application/octet-Stream"
//	}
//	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
//	switch ext {
//	case "jpg", "jpeg":
//		return "image/jpeg"
//	case "png":
//		return "image/png"
//	case "gif":
//		return "image/gif"
//	case "pdf":
//		return "application/pdf"
//	case "txt":
//		return "text/plain"
//	case "md":
//		return "text/markdown"
//	case "csv":
//		return "text/csv"
//	case "json":
//		return "application/json"
//	case "xml":
//		return "application/xml"
//	case "html":
//		return "text/html"
//	case "yaml", "yml":
//		return "application/x-yaml"
//	case "zip":
//		return "application/zip"
//	case "tar":
//		return "application/x-tar"
//	case "mp3":
//		return "audio/mpeg"
//	case "mp4":
//		return "video/mp4"
//	}
//	return "application/octet-Stream"
//}

package agent

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/pkg/agently/conversation"
)

func (s *Service) summarizeIfNeeded(ctx context.Context, input *QueryInput, conv *apiconv.Conversation) error {
	if input.ShallAutoSummarize() {
		if err := s.Summarize(ctx, conv); err != nil {
			return err
		}
	}
	return nil
}

//go:embed summary.md
var summaryPrompt string

func (s *Service) Summarize(ctx context.Context, conv *apiconv.Conversation) error {
	transcript := conv.GetTranscript()
	if !(conv.Summary == nil || *conv.Summary == "") {
		transcript = transcript.Last()
		summary := "SUMMARY:" + *conv.Summary
		transcript[0].Message = append(transcript[0].Message, &conversation.MessageView{
			Role:    "user",
			Type:    "text",
			Content: &summary,
		})
	}

	for i := range transcript {
		messages := transcript[i].Filter(func(v *apiconv.Message) bool {
			if v == nil || v.IsArchived() || v.IsInterim() || v.Content == nil || *v.Content == "" {
				return false
			}
			return true
		})
		transcript[i].SetMessages(messages)
	}

	bindings := prompt.Binding{}
	if err := s.BuildHistory(ctx, transcript, &bindings); err != nil {
		return err
	}
	genInput := &core.GenerateInput{
		Binding: &bindings,
		UserID:  "system",
		Prompt: &prompt.Prompt{
			Text: summaryPrompt,
		},
	}
	if conv.DefaultModel != nil {
		genInput.Model = *conv.DefaultModel
	}
	if genInput.Options == nil {
		genInput.Options = &llm.Options{}
	}
	output := &core.GenerateOutput{}
	if err := s.llm.Generate(ctx, genInput, output); err != nil {
		return err
	}

	lines := strings.Split(output.Content, "\n")
	if len(lines) == 0 {
		return nil
	}
	title := lines[0]
	body := strings.Join(lines[1:], "\n")

	updatedConv := apiconv.NewConversation()
	updatedConv.SetId(conv.Id)
	updatedConv.SetTitle(title)
	updatedConv.SetSummary(body)
	if err := s.conversation.PatchConversations(ctx, updatedConv); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	return nil
}

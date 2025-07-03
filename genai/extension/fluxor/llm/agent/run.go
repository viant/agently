package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/summary"
	"github.com/viant/fluxor/model/types"
)

// RunInput describes the parameters for the "run" executable that spawns a
// sub-agent in a child conversation.
type RunInput struct {
	ConversationID  string         `json:"conversationId,omitempty"`
	AgentName       string         `json:"agentName"`
	Query           string         `json:"query"`
	Context         map[string]any `json:"context,omitempty"`
	ElicitationMode string         `json:"elicitationMode,omitempty"`
	Visibility      string         `json:"visibility,omitempty"` // full|summary|none
	Title           string         `json:"title,omitempty"`
}

type RunOutput struct {
	ConversationID string `json:"conversationId"`
}

// run implements the executable registered under "run" on Service.
func (s *Service) run(ctx context.Context, in, out interface{}) error {
	arg, ok := in.(*RunInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	res, ok := out.(*RunOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	parentID := memory.ConversationIDFromContext(ctx)

	childID := strings.TrimSpace(arg.ConversationID)
	writeLink := false
	if childID == "" {
		childID = memory.NewChildConversation(ctx, s.history, parentID, arg.Title, arg.Visibility)
		writeLink = true
	}
	res.ConversationID = childID

	// Delegate to regular Query processing in the child conversation.
	qi := &QueryInput{
		ConversationID:  childID,
		AgentName:       arg.AgentName,
		Query:           arg.Query,
		Context:         arg.Context,
		ElicitationMode: arg.ElicitationMode,
	}
	if _, err := s.Query(ctx, qi); err != nil {
		return err
	}

	// Write link (and optional summary) to parent thread.
	if writeLink && parentID != "" {
		content := "↪ " + arg.Title

		if strings.EqualFold(arg.Visibility, "summary") && s.llm != nil {
			model := s.runSummaryModel
			if strings.TrimSpace(model) == "" && qi.Agent != nil {
				model = qi.Agent.Model
			}
			if sum, err := summary.Summarize(ctx, s.history, s.llm, model, childID, s.lastN, s.summaryPrompt); err == nil && strings.TrimSpace(sum) != "" {
				content += "\n" + sum
			}
		}

		_ = s.history.AddMessage(ctx, memory.Message{
			ID:             uuid.New().String(),
			ConversationID: parentID,
			Role:           "assistant",
			Actor:          "agent.run",
			Content:        content,
			CallbackURL:    childID,
		})
	}
	return nil
}

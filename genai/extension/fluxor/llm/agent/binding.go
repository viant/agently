package agent

import (
	"context"
	"strings"

	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	base "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	padapter "github.com/viant/agently/genai/prompt/adapter"
	msgread "github.com/viant/agently/internal/dao/message/read"
	d "github.com/viant/agently/internal/domain"
	"github.com/viant/fluxor/model/types"
)

func (s *Service) BuildBinding(ctx context.Context, input *QueryInput, isSystem bool) (*prompt.Binding, error) {
	if input == nil || input.Agent == nil {
		return nil, types.NewInvalidInputError(input)
	}
	b := &prompt.Binding{}
	b.Task = s.buildTaskBinding(input)
	if hist, err := s.buildHistoryBinding(ctx, input); err == nil {
		b.History = hist
	}
	if sig, _, err := s.buildToolSignatures(ctx, input); err != nil {
		return nil, err
	} else if len(sig) > 0 {
		b.Tools.Signatures = sig
		// Determine native tool-use capability from the selected model.
		model := ""
		if input.Agent != nil {
			model = input.Agent.Model
		}
		if strings.TrimSpace(input.ModelOverride) != "" {
			model = strings.TrimSpace(input.ModelOverride)
		}
		b.Flags.CanUseTool = s.llm != nil && s.llm.ModelImplements(ctx, model, base.CanUseTools)
	}
	if execs, err := s.buildToolExecutions(ctx, input); err != nil {
		return nil, err
	} else if len(execs) > 0 {
		b.Tools.Executions = execs
	}
	docs, err := s.buildDocumentsBinding(ctx, input, false)
	if err != nil {
		return nil, err
	}
	b.Documents = docs

	b.SystemDocuments, err = s.buildDocumentsBinding(ctx, input, true)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (s *Service) buildTaskBinding(input *QueryInput) prompt.Task {
	return prompt.Task{Prompt: input.Query}
}

func (s *Service) buildHistoryBinding(ctx context.Context, input *QueryInput) (prompt.History, error) {
	var h prompt.History
	convID := s.conversationID(input)
	if convID == "" {
		return h, nil
	}
	turnID := memory.TurnIDFromContext(ctx)
	var (
		views []*msgread.MessageView
		err   error
	)

	if turnID != "" {
		views, err = s.store.Messages().GetTranscript(ctx, convID, msgread.WithTurnID(turnID))
	} else {
		// Use conversation-level normalized transcript and filter to history
		views, err = s.store.Messages().GetTranscript(ctx, convID)
	}
	if err != nil {
		return h, err
	}
	var flat []*prompt.Message
	for _, v := range d.Transcript(views).History() {
		if v == nil {
			continue
		}
		if strings.TrimSpace(v.Content) == "" {
			continue
		}
		flat = append(flat, &prompt.Message{Role: v.Role, Content: v.Content})
	}
	if s.lastN > 0 && len(flat) > s.lastN {
		flat = flat[len(flat)-s.lastN:]
	}
	h.Messages = flat
	return h, nil
}

func (s *Service) buildToolSignatures(ctx context.Context, input *QueryInput) ([]*llm.ToolDefinition, bool, error) {
	if s.registry == nil || input.Agent == nil || len(input.Agent.Tool) == 0 {
		return nil, false, nil
	}
	tools, err := s.resolveTools(input, true)
	if err != nil {
		return nil, false, err
	}
	out := padapter.ToToolDefinitions(tools)
	return out, len(out) > 0, nil
}

func (s *Service) buildToolExecutions(ctx context.Context, input *QueryInput) ([]*llm.ToolCall, error) {
	if s.store == nil || s.store.Messages() == nil {
		return nil, nil
	}
	convID := s.conversationID(input)
	turnID := memory.TurnIDFromContext(ctx)
	if convID == "" || turnID == "" {
		return nil, nil
	}
	views, err := s.store.Messages().GetTranscript(ctx, convID, msgread.WithTurnID(turnID), msgread.WithIncludeOutcomes())
	if err != nil {
		return nil, err
	}
	var execs []*llm.ToolCall
	for _, v := range views {
		if v == nil || len(v.Executions) == 0 {
			continue
		}
		execs = append(execs, padapter.ToolCallsFromOutcomes(v.Executions)...)
	}
	if len(execs) > 0 {
		return execs, nil
	}
	for _, v := range views {
		if v == nil || strings.ToLower(v.Role) != "tool" {
			continue
		}
		name := ""
		if v.ToolCall != nil {
			name = v.ToolCall.ToolName
		}
		errMsg := ""
		if v.ToolCall != nil && v.ToolCall.ErrorMessage != nil {
			errMsg = *v.ToolCall.ErrorMessage
		}
		summary := trimStr(v.Content, 160)
		execs = append(execs, &llm.ToolCall{
			Name:   name,
			Result: summary,
			Error:  errMsg,
		})
	}
	return execs, nil
}

func (s *Service) buildDocumentsBinding(ctx context.Context, input *QueryInput, isSystem bool) (prompt.Documents, error) {
	var docs prompt.Documents
	var knowledge []*agent.Knowledge
	if isSystem {
		knowledge = input.Agent.SystemKnowledge
	} else {
		knowledge = input.Agent.Knowledge
	}
	matchedDocs, err := s.matchDocuments(ctx, input, knowledge)
	if err != nil {
		return docs, err
	}
	docs.Items = padapter.FromSchemaDocs(matchedDocs)
	return docs, nil
}

// trimStr ensures s is at most n runes, appending ellipsis when truncated.
func trimStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	if n > 3 {
		return s[:n-3] + "..."
	}
	return s[:n]
}

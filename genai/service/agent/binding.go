package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/viant/agently/genai/agent"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/llm"
	base "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	padapter "github.com/viant/agently/genai/prompt/adapter"
	msgread "github.com/viant/agently/internal/dao/message/read"
	d "github.com/viant/agently/internal/domain"
)

func (s *Service) BuildBinding(ctx context.Context, input *QueryInput) (*prompt.Binding, error) {
	b := &prompt.Binding{}
	b.Task = s.buildTaskBinding(input)
	if hist, err := s.buildHistoryBinding(ctx, input); err == nil {
		b.History = hist
	}

	sig, _, err := s.buildToolSignatures(input)
	if err != nil {
		return nil, err
	}

	if len(sig) > 0 {
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
	b.Context = input.Context
	return b, nil
}

func (s *Service) buildTaskBinding(input *QueryInput) prompt.Task {
	return prompt.Task{Prompt: input.Query}
}

func (s *Service) buildHistoryBinding(ctx context.Context, input *QueryInput) (prompt.History, error) {
	var h prompt.History
	convID := input.ConversationID
	// Use conversation-level normalized transcript and filter to history
	views, err := s.store.Messages().GetTranscript(ctx, convID)
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
	// Avoid duplicating the current user query in both Task.Prompt and History:
	// if the most recent history entry is a user message equal to input.Query,
	// drop it from History so the prompt shows it only once under "User Query".
	if n := len(flat); n > 0 {
		last := flat[n-1]
		if last != nil && strings.EqualFold(strings.TrimSpace(last.Role), "user") &&
			strings.TrimSpace(last.Content) == strings.TrimSpace(input.Query) {
			flat = flat[:n-1]
		}
	}
	h.Messages = flat
	return h, nil
}

func (s *Service) buildToolSignatures(input *QueryInput) ([]*llm.ToolDefinition, bool, error) {
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
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("failed to get turn meta from context")
	}
	views, err := s.store.Messages().GetTranscript(ctx, turn.ConversationID, msgread.WithTurnID(turn.TurnID))
	if err != nil {
		return nil, err
	}
	outcome, err := d.BuildToolOutcomes(ctx, s.store, d.Transcript(views))
	if err != nil {
		return nil, err
	}
	if outcome == nil || len(outcome.Steps) == 0 {
		return nil, nil
	}
	calls := padapter.ToolCallsFromOutcomes([]*plan.Outcome{outcome})
	return calls, nil
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

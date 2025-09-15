package agent

import (
	"context"
	"encoding/json"
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
	apiconv "github.com/viant/agently/sdk/conversation"
)

func (s *Service) BuildBinding(ctx context.Context, input *QueryInput) (*prompt.Binding, error) {
	b := &prompt.Binding{}
	b.Task = s.buildTaskBinding(input)
	// Fetch conversation transcript once and reuse; bubble up errors
	if s.convAPI == nil {
		return nil, fmt.Errorf("conversation API not configured")
	}
	conv, err := s.convAPI.Get(ctx, input.ConversationID)
	if err != nil {
		return nil, err
	}
	hist, err := s.buildHistoryBindingFromTranscript(ctx, input, conv)
	if err != nil {
		return nil, err
	}
	b.History = hist

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
	if execs, err := s.buildToolExecutionsFromTranscript(ctx, input, conv); err != nil {
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
	// This method delegates to buildHistoryBindingFromTranscript with no pre-fetched conversation,
	// preserving backward compatibility when called directly.
	return s.buildHistoryBindingFromTranscript(ctx, input, nil)
}

// buildHistoryBindingFromTranscript derives history from a provided conversation (if non-nil),
// otherwise falls back to DAO transcript for compatibility.
func (s *Service) buildHistoryBindingFromTranscript(ctx context.Context, input *QueryInput, conv *apiconv.Conversation) (prompt.History, error) {

	var h prompt.History
	if conv == nil {
		return h, nil
	}
	transcript := conv.GetTranscript()
	h.Messages = transcript.History(input.Query)
	return h, nil
}

// buildToolExecutionsFromTranscript extracts tool calls from the provided conversation transcript for the current turn.
func (s *Service) buildToolExecutionsFromTranscript(ctx context.Context, input *QueryInput, conv *apiconv.Conversation) ([]*llm.ToolCall, error) {
	if conv == nil {
		return s.buildToolExecutions(ctx, input)
	}
	turnMeta, ok := memory.TurnMetaFromContext(ctx)
	if !ok || strings.TrimSpace(turnMeta.TurnID) == "" {
		return nil, nil
	}
	transcript := conv.GetTranscript()
	// Find current turn
	var aTurn *apiconv.Turn
	for _, t := range transcript {
		if t != nil && t.Id == turnMeta.TurnID {
			aTurn = t
			break
		}
	}
	if aTurn == nil {
		return nil, nil
	}
	// Build tool calls from messages in current turn
	var out []*llm.ToolCall
	for _, m := range aTurn.ToolCalls() {
		args := map[string]interface{}{}
		// Prefer request payload (inline body JSON) for arguments
		if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
			raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
			if raw != "" {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
					args = parsed
				}
			}
		}
		tc := &llm.ToolCall{ID: m.ToolCall.OpId, Name: name, Arguments: args}
		out = append(out, tc)
	}
	return out, nil
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

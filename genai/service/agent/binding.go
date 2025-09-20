package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	base "github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	padapter "github.com/viant/agently/genai/prompt/adapter"
)

func (s *Service) BuildBinding(ctx context.Context, input *QueryInput) (*prompt.Binding, error) {
	b := &prompt.Binding{}
	b.Task = s.buildTaskBinding(input)
	// Fetch conversation transcript once and reuse; bubble up errors
	if s.convClient == nil {
		return nil, fmt.Errorf("conversation API not configured")
	}
	conv, err := s.convClient.GetConversation(ctx, input.ConversationID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return nil, err
	}
	hist, err := s.buildHistory(ctx, input, conv)
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
	if execs, err := s.buildToolExecutions(ctx, input, conv); err != nil {
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
	return prompt.Task{Prompt: input.Query, Attachments: input.Attachments}
}

// buildHistory derives history from a provided conversation (if non-nil),
// otherwise falls back to DAO transcript for compatibility.
func (s *Service) buildHistory(ctx context.Context, input *QueryInput, conv *apiconv.Conversation) (prompt.History, error) {

	var h prompt.History
	if conv == nil {
		return h, nil
	}
	transcript := conv.GetTranscript()
	h.Messages = transcript.History(input.Query)
	return h, nil
}

// buildToolExecutions extracts tool calls from the provided conversation transcript for the current turn.
func (s *Service) buildToolExecutions(ctx context.Context, input *QueryInput, conv *apiconv.Conversation) ([]*llm.ToolCall, error) {
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
		result := ""
		if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
			result = strings.TrimSpace(*m.ToolCall.ResponsePayload.InlineBody)
		}
		tc := llm.NewToolCall(m.ToolCall.OpId, m.ToolCall.ToolName, args, result)
		out = append(out, &tc)
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

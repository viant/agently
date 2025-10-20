package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
)

type ListCandidatesInput struct {
	// Max number of candidates to return (default: 50)
	Limit int `json:"limit,omitempty" description:"Max number of candidates to return (default 50)."`
	// Types filter (user, assistant, tool). Empty = all
	Types []string `json:"types,omitempty" description:"Optional types to include: user, assistant, tool."`
}

type Candidate struct {
	MessageID string `json:"messageId"`
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	Preview   string `json:"preview"`
	ByteSize  int    `json:"byteSize"`
	TokenSize int    `json:"tokenSize"`
}

type ListCandidatesOutput struct {
	Candidates []Candidate `json:"candidates"`
}

func (s *Service) listCandidates(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ListCandidatesInput)
	if !ok {
		return fmt.Errorf("invalid input")
	}
	output, ok := out.(*ListCandidatesOutput)
	if !ok {
		return fmt.Errorf("invalid output")
	}
	if s == nil || s.conv == nil {
		return fmt.Errorf("conversation client not initialised")
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		return fmt.Errorf("missing conversation id in context")
	}
	conv, err := s.conv.GetConversation(ctx, convID, apiconv.WithIncludeToolCall(true))
	if err != nil || conv == nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	// Identify last user message id to exclude
	lastUserID := ""
	tr := conv.GetTranscript()
	for i := len(tr) - 1; i >= 0 && lastUserID == ""; i-- {
		t := tr[i]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		for j := len(t.Message) - 1; j >= 0; j-- {
			m := t.Message[j]
			if m == nil || m.Interim != 0 || m.Content == nil || strings.TrimSpace(*m.Content) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
				lastUserID = m.Id
				break
			}
		}
	}
	max := input.Limit
	if max <= 0 {
		max = 50
	}
	typeFilter := map[string]bool{}
	for _, t := range input.Types {
		typeFilter[strings.ToLower(strings.TrimSpace(t))] = true
	}
	var candidates []Candidate
	appendCandidate := func(c Candidate) {
		if len(candidates) < max {
			candidates = append(candidates, c)
		}
	}
	for _, t := range tr {
		if t == nil {
			continue
		}
		for _, m := range t.Message {
			if m == nil || m.Id == lastUserID || (m.Archived != nil && *m.Archived == 1) || m.Interim != 0 {
				continue
			}
			typ := strings.ToLower(strings.TrimSpace(m.Type))
			role := strings.ToLower(strings.TrimSpace(m.Role))
			// Skip non-text non-tool
			if typ != "text" && m.ToolCall == nil {
				continue
			}
			if len(typeFilter) > 0 {
				cat := typ
				if m.ToolCall != nil {
					cat = "tool"
				} else {
					cat = role
				}
				if !typeFilter[cat] {
					continue
				}
			}
			if m.ToolCall != nil {
				// Tool candidate
				toolName := strings.TrimSpace(m.ToolCall.ToolName)
				// args preview
				var args map[string]interface{}
				if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
					raw := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
					if raw != "" {
						var parsed map[string]interface{}
						_ = json.Unmarshal([]byte(raw), &parsed)
						args = parsed
					}
				}
				argStr, _ := json.Marshal(args)
				ap := string(argStr)
				if len(ap) > 100 {
					ap = ap[:100]
				}
				body := ""
				if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
					body = *m.ToolCall.ResponsePayload.InlineBody
				}
				c := Candidate{MessageID: m.Id, Type: "tool", ToolName: toolName, Preview: ap, ByteSize: len(body), TokenSize: estimateTokens(body)}
				appendCandidate(c)
				continue
			}
			if typ == "text" && (role == "user" || role == "assistant") {
				body := ""
				if m.Content != nil {
					body = *m.Content
				}
				pv := body
				if len(pv) > 100 {
					pv = pv[:100]
				}
				c := Candidate{MessageID: m.Id, Type: role, Role: role, Preview: pv, ByteSize: len(body), TokenSize: estimateTokens(body)}
				appendCandidate(c)
			}
			if len(candidates) >= max {
				break
			}
		}
		if len(candidates) >= max {
			break
		}
	}
	output.Candidates = candidates
	return nil
}

// estimateTokens provides a simple character-based token estimate heuristic.
// estimateTokens is defined in tokens.go

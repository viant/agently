package service

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
	"time"

	"github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/tool"
)

// ChatRequest and ChatResponse remain same
type ChatRequest struct {
	ConversationID string
	AgentPath      string
	Query          string

	Policy  *tool.Policy
	Timeout time.Duration
}

type ChatResponse struct {
	ConversationID string
	Content        string
	Elicitation    *plan.Elicitation
	Plan           *plan.Plan
}

// Chat executes an interactive turn, optionally looping on elicitation when
// an InteractionHandler is configured.
func (s *Service) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if s == nil || s.exec == nil {
		return nil, nil
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	if req.Policy != nil {
		ctx = tool.WithPolicy(ctx, req.Policy)
	}

	turn := func(ctx context.Context, convID, query string) (*agent.QueryOutput, string, error) {
		input := &agent.QueryInput{ConversationID: convID, AgentName: req.AgentPath, Query: query}
		out, err := s.exec.Conversation().Accept(ctx, input)
		return out, input.ConversationID, err
	}

	convID := req.ConversationID
	currentQuery := req.Query

	var out *agent.QueryOutput
	var err error

	for {
		var newID string
		out, newID, err = turn(ctx, convID, currentQuery)
		if err != nil {
			return nil, err
		}
		if newID != "" {
			convID = newID
		}

		// no elicitation or no handler – finish.
		if out.Elicitation == nil || out.Elicitation.IsEmpty() || s.opts.Interaction == nil {
			break
		}

		res, err := s.opts.Interaction.Accept(ctx, out.Elicitation)
		if err != nil {
			return nil, err
		}

		switch res.Action {
		case ActionAccept:
			if len(res.Payload) == 0 {
				// nothing to send back -> stop
				goto DONE
			}
			currentQuery = string(res.Payload)
			continue // loop again
		case ActionDecline, ActionTimeout:
			goto DONE
		default:
			goto DONE
		}
	}

DONE:
	return &ChatResponse{ConversationID: convID, Content: out.Content, Elicitation: out.Elicitation, Plan: out.Plan}, nil
}

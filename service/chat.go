package service

import (
	"context"
	"time"

	"github.com/viant/agently/genai/agent/plan"
	execpkg "github.com/viant/agently/genai/executor"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/tool"
)

// InteractionHandler defines a pluggable mechanism that can satisfy an
// elicitation request originating from the assistant.  It is optional – when
// nil the service returns the elicitation to the caller for custom handling.
type InteractionHandler interface {
	// Accept receives the elicitation description and should return the JSON
	// payload to resubmit along with a flag indicating whether the chat flow
	// should continue automatically.
	Accept(ctx context.Context, el *plan.Elicitation) (payload []byte, proceed bool, err error)
}

// Options provides optional knobs that influence the service behaviour.
type Options struct {
	Interaction InteractionHandler
}

// Service exposes high-level operations (currently Chat) that are decoupled
// from any particular user-interface.
type Service struct {
	exec *execpkg.Service
	opts Options
}

// New returns a Service using the supplied executor.Service. Ownership of
// exec is left to the caller – Service does not Stop()/Shutdown() it.
func New(exec *execpkg.Service, opts Options) *Service {
	return &Service{exec: exec, opts: opts}
}

// ChatRequest holds input parameters for a single user turn.
type ChatRequest struct {
	ConversationID string
	AgentPath      string
	Query          string // user utterance or JSON payload for elicitation

	Policy  *tool.Policy  // nil = default (auto)
	Timeout time.Duration // 0 = no timeout
}

// ChatResponse is produced after the assistant responds.
type ChatResponse struct {
	ConversationID string
	Content        string
	Elicitation    *plan.Elicitation
}

// Chat performs the exchange. If an InteractionHandler was configured and the
// assistant requests additional information (elicitation) the handler is
// invoked and – when it returns `proceed=true` – the conversation continues
// automatically until no further elicitation is required.  Without handler the
// first assistant elicitation is returned to the caller.
func (s *Service) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if s == nil || s.exec == nil {
		return nil, nil // caller error – avoid panic
	}

	// Apply timeout if requested.
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Execute one turn ---------------------------------------------------
	turn := func(ctx context.Context, convID, query string) (*agentpkg.QueryOutput, string, error) {
		input := &agentpkg.QueryInput{
			ConversationID: convID,
			Location:       req.AgentPath,
			Query:          query,
		}
		out, err := s.exec.Conversation().Accept(ctx, input)
		return out, input.ConversationID, err
	}

	convID := req.ConversationID
	var out *agentpkg.QueryOutput
	var err error

	currentQuery := req.Query

	for {
		var newConvID string
		out, newConvID, err = turn(ctx, convID, currentQuery)
		if err != nil {
			return nil, err
		}

		if newConvID != "" {
			convID = newConvID
		}

		// no elicitation – done.
		if out.Elicitation == nil || out.Elicitation.IsEmpty() {
			break
		}

		// If no interaction handler configured -> return early.
		if s.opts.Interaction == nil {
			break
		}

		payload, proceed, err := s.opts.Interaction.Accept(ctx, out.Elicitation)
		if err != nil || !proceed {
			break // bubble to caller; include elicitation in response
		}

		if len(payload) == 0 {
			break // nothing to submit -> stop
		}

		currentQuery = string(payload)
		// Continue loop with new query, same convID.
	}

	res := &ChatResponse{
		ConversationID: convID,
		Content:        out.Content,
		Elicitation:    out.Elicitation,
	}
	return res, nil
}

// inputConvID extracts the conversationID from QueryOutput when not available
// on the top-level Agent.Source (backward-compat helper).

package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	convw "github.com/viant/agently/internal/dao/conversation/write"
	"github.com/viant/agently/internal/templating"
	"github.com/viant/fluxor/model/types"
)

// RunInput describes the parameters for the "run" executable that spawns a
// sub-agent in a child conversation.
type RunInput struct {
	ConversationID  string         `json:"conversationId,omitempty"`
	AgentName       string         `json:"agentName"`
	Query           string         `json:"query"`
	QueryTemplate   string         `json:"queryTemplate,omitempty"`
	Context         map[string]any `json:"context,omitempty"`
	ElicitationMode string         `json:"elicitationMode,omitempty"`
	Visibility      string         `json:"visibility,omitempty"` // full|summary|none
	Title           string         `json:"title,omitempty"`
}

type RunOutput struct {
	ConversationID string `json:"conversationId"`
	Content        string `json:"content,omitempty"`
	Answer         string `json:"answer,omitempty"`
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
		childID = uuid.NewString()
		// Create conversation via domain store when available
		if s.store != nil && s.store.Conversations() != nil {
			cw := &convw.Conversation{Has: &convw.ConversationHas{}}
			cw.SetId(childID)
			if strings.TrimSpace(arg.AgentName) != "" {
				cw.SetAgentName(arg.AgentName)
			}
			if strings.TrimSpace(arg.Title) != "" {
				cw.SetTitle(arg.Title)
			}
			if strings.TrimSpace(arg.Visibility) != "" {
				cw.SetVisibility(arg.Visibility)
			}
			if _, err := s.store.Conversations().Patch(ctx, cw); err != nil {
				return err
			}
		}
		writeLink = true
	}
	res.ConversationID = childID

	// Resolve final query – support optional velty-based QueryTemplate that
	// can interpolate Context and the original Query (as ${Content}).
	finalQuery := arg.Query

	if strings.TrimSpace(arg.QueryTemplate) != "" {
		vars := map[string]interface{}{}
		if arg.Context != nil {
			for k, v := range arg.Context {
				vars[k] = v
			}
		}
		vars["Content"] = arg.Query
		rendered, err := templating.Expand(arg.QueryTemplate, vars)
		if err != nil {
			return err
		}
		finalQuery = rendered
	}

	// Delegate to regular Query processing in the child conversation.
	qi := &QueryInput{
		ConversationID:  childID,
		AgentName:       arg.AgentName,
		Query:           finalQuery,
		Context:         arg.Context,
		ElicitationMode: arg.ElicitationMode,
	}
	queryResp, err := s.Query(ctx, qi)
	if err != nil {
		return err
	}
	res.Answer = queryResp.Content

	// Write link (and optional summary) to parent thread.
	if writeLink && parentID != "" {
		content := "↪ " + arg.Title
		// v1: omit auto-summary; policy-driven summarization can be added later
		msg := memory.Message{ID: uuid.NewString(), ConversationID: parentID, Role: "assistant", Actor: "agent.run", Content: content}
		s.recorder.RecordMessage(ctx, msg)
		res.Content = content
	}
	return nil
}

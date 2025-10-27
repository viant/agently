package integrate

import (
	"context"
	"fmt"

	apiconv "github.com/viant/agently/client/conversation"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation"
	"github.com/viant/agently/genai/memory"
)

// McpOOBElicitationPrompt uses Agently's elicitation service and conversation
// client to record an out-of-band interaction message instructing the user to
// open the authorization URL in a browser. It does not block for completion.
type McpOOBElicitationPrompt struct {
	Elic *elicitation.Service
	Conv apiconv.Client
}

func (p *McpOOBElicitationPrompt) PromptOOB(ctx context.Context, authorizationURL string, meta OAuthMeta) error {
	if p == nil || p.Elic == nil || p.Conv == nil || authorizationURL == "" || meta.ConversationID == "" {
		return nil
	}
	// Build turn meta using last turn id if available
	var turn memory.TurnMeta
	turn.ConversationID = meta.ConversationID
	if cv, err := p.Conv.GetConversation(ctx, meta.ConversationID); err == nil && cv != nil && cv.LastTurnId != nil {
		turn.TurnID = *cv.LastTurnId
		turn.ParentMessageID = *cv.LastTurnId
	}
	// Compose a minimal OOB instruction. UI renders this as a regular message; advanced
	// UIs can interpret and provide a clickable link.
	provider := meta.ProviderName
	if provider == "" {
		provider = meta.Origin
	}
	msg := fmt.Sprintf("To continue, sign in with %s by opening: %s", provider, authorizationURL)
	req := &plan.Elicitation{}
	req.ElicitRequestParams.Message = msg
	// Record as assistant text so it appears immediately; status defaults to pending via service.Record
	_, _ = p.Elic.Record(ctx, &turn, "assistant", req)
	return nil
}

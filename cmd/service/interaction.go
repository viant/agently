package service

import (
	"context"
	"encoding/json"

	"github.com/viant/agently/genai/agent/plan"
)

// InteractionAction enumerates the high-level outcome of an interaction.
type InteractionAction string

const (
	ActionAccept  InteractionAction = "accept"  // user provided payload
	ActionDecline InteractionAction = "decline" // user explicitly declined
	ActionTimeout InteractionAction = "timeout" // no answer until ctx done
)

// AcceptResult summarises the result of asking the user.
//
// Action    – what the user decided (accept/decline/timeout).
// Payload   – JSON document to resubmit when Action==accept.
// RedirectURL – optional browser URL (useful for MCP UA flows).
type AcceptResult struct {
	Action      InteractionAction
	Payload     json.RawMessage
	RedirectURL string
}

// InteractionHandler resolves an elicitation request through any UX channel.
// Implementations may block (CLI) or return immediately (HTTP).
type InteractionHandler interface {
	Accept(ctx context.Context, el *plan.Elicitation) (AcceptResult, error)
}

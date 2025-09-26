package cancel

import "context"

// Registry exposes registration of per-turn cancellation functions so that
// external actors (e.g., HTTP endpoints) can cancel a running turn without
// creating additional cancellation scopes in the lower layers.
//
// Implementations should store one or more cancel funcs keyed by (convID, turnID)
// and remove them on Complete. They should also support aborting a single turn
// or all turns within a conversation.
type Registry interface {
	Register(convID, turnID string, cancel context.CancelFunc)
	Complete(convID, turnID string, cancel context.CancelFunc)

	CancelTurn(turnID string) bool
	CancelConversation(convID string) bool
}

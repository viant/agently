package tool

import "context"

// DAO defines persistence operations for tool calls. Implementations live in
// adapter layers (e.g. pkg/agently/tool) and should remain independent from
// business logic.
type DAO interface {
	// Add persists a tool call record.
	Add(ctx context.Context, call *Call) error

	// List returns tool-call records  filtered by conversationID.
	List(ctx context.Context, conversationID string) ([]*Call, error)
}

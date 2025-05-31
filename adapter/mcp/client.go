package mcp

import (
	"context"
	"github.com/viant/jsonrpc"
	"github.com/viant/mcp-protocol/schema"
)

// Implementer is a default implementation of the Operations interface.
type Implementer struct{}

func (c *Implementer) OnNotification(ctx context.Context, notification *jsonrpc.Notification) {
	// Default implementation does nothing
}

func (c *Implementer) Implements(method string) bool {
	// Default implementation returns false, indicating the method is not implemented
	return false
}

func (c *Implementer) ListRoots(ctx context.Context, params *schema.ListRootsRequestParams) (*schema.ListRootsResult, *jsonrpc.Error) {
	return nil, nil
}

func (c *Implementer) CreateMessage(ctx context.Context, params *schema.CreateMessageRequestParams) (*schema.CreateMessageResult, *jsonrpc.Error) {
	// Default implementation returns nil, indicating no result
	return nil, nil
}

func NewClient(options ...Option) *Implementer {
	// Create a new instance of Implementer
	ret := &Implementer{}
	for _, option := range options {
		// Apply each option to the Implementer instance
		option(ret)
	}
	return ret
}

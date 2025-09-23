package router

import "github.com/viant/mcp-protocol/schema"

// noopRouter implements ElicitationRouter but does not coordinate any channels.
type noopRouter struct{}

func (noopRouter) RegisterByElicitationID(convID, elicID string, ch chan *schema.ElicitResult) {}
func (noopRouter) RemoveByElicitation(convID, elicID string)                                   {}
func (noopRouter) AcceptByElicitation(convID, elicID string, res *schema.ElicitResult) bool {
	return false
}

// NewNoop returns a no-op router implementation.
func NewNoop() ElicitationRouter { return noopRouter{} }

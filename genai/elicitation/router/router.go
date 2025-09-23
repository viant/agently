package router

import (
	mcprouter "github.com/viant/agently/adapter/mcp/router"
	"github.com/viant/mcp-protocol/schema"
)

// ElicitationRouter is the minimal interface used by agent/HTTP to coordinate
// elicitation wait/unblock across assistant/tool flows.
type ElicitationRouter interface {
	RegisterByElicitationID(convID, elicID string, ch chan *schema.ElicitResult)
	RemoveByElicitation(convID, elicID string)
	AcceptByElicitation(convID, elicID string, res *schema.ElicitResult) bool
}

// Router aliases the MCP router implementation so adopters can import a
// neutral package name while reusing the production implementation.
type Router = mcprouter.Router

// New returns a new router instance.
func New() *Router { return mcprouter.New() }

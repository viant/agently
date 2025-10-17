package tool

import (
	"github.com/viant/agently/genai/agent"
	svc "github.com/viant/agently/genai/tool/service"
	"github.com/viant/agently/internal/mcp/manager"
	internal "github.com/viant/agently/internal/tool/registry"
)

// NewDefaultRegistry constructs the default MCP-backed tool registry with built-ins.
func NewDefaultRegistry(mgr *manager.Manager) (Registry, error) { return internal.NewWithManager(mgr) }

// InjectVirtualAgentTools exposes agents as virtual tools when supported by the registry implementation.
func InjectVirtualAgentTools(reg Registry, agents []*agent.Agent, domain string) {
	type injector interface{ InjectVirtualAgentTools([]*agent.Agent, string) }
	if v, ok := reg.(injector); ok {
		v.InjectVirtualAgentTools(agents, domain)
	}
}

// AddInternalService attempts to register a service as an internal MCP client on the default registry.
func AddInternalService(reg Registry, s svc.Service) {
	type adder interface{ AddInternalService(s svc.Service) error }
	if v, ok := reg.(adder); ok {
		_ = v.AddInternalService(s) // bubble up errors to debug logs in registry if any
	}
}

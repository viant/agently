package hotswap

import (
	"context"

	coreagent "github.com/viant/agently/genai/agent"
	agentfinder "github.com/viant/agently/internal/finder/agent"
	agentloader "github.com/viant/agently/internal/loader/agent"
)

// NewAgentAdaptor wires together the agent loader and finder so that workspace
// file changes picked up by the HotSwap manager seamlessly refresh the
// in-memory cache used by the executor.
func NewAgentAdaptor(loader *agentloader.Service, finder *agentfinder.Finder) Reloadable {
	if loader == nil {
		panic("hotswap: nil agent loader")
	}
	if finder == nil {
		panic("hotswap: nil agent finder")
	}

	loadFn := func(ctx context.Context, name string) (*coreagent.Agent, error) {
		return loader.Load(ctx, name)
	}

	setFn := func(name string, a *coreagent.Agent) { finder.Add(name, a) }

	removeFn := finder.Remove

	return NewAdaptor[*coreagent.Agent](loadFn, setFn, removeFn)
}

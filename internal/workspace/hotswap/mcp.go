package hotswap

import (
	"context"

	"github.com/viant/afs"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
)

// NewMCPAdaptor hot-swaps MCP client option YAMLs. It reloads the file into
// the provided Group so that new orchestrations or restarts see the updated
// servers.
//
// Note: In decoupled mode this only refreshes workspace definitions. Per‑process
// MCP clients are refreshed on next use or via explicit restart.
func NewMCPAdaptor(group *mcpcfg.Group[*mcpcfg.MCPClient]) Reloadable {
	if group == nil {
		panic("hotswap: nil MCP group")
	}

	repo := mcprepo.New(afs.New())

	loadFn := func(ctx context.Context, name string) (*mcpcfg.MCPClient, error) {
		return repo.Load(ctx, name)
	}

	setFn := func(name string, opt *mcpcfg.MCPClient) {
		// Replace existing by Name or append.
		replaced := false
		for i, it := range group.Items {
			if it != nil && it.Name == opt.Name {
				group.Items[i] = opt
				replaced = true
				break
			}
		}
		if !replaced {
			group.Items = append(group.Items, opt)
		}
	}

	removeFn := func(name string) {
		idx := -1
		for i, it := range group.Items {
			if it != nil && it.Name == name {
				idx = i
				break
			}
		}
		if idx >= 0 {
			group.Items = append(group.Items[:idx], group.Items[idx+1:]...)
		}
	}

	return NewAdaptor[*mcpcfg.MCPClient](loadFn, setFn, removeFn)
}

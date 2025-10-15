package mcpfs

import (
	"context"

	"github.com/viant/afs"
	"github.com/viant/afs/storage"
	mcpmgr "github.com/viant/agently/adapter/mcp/manager"
	"github.com/viant/agently/internal/mcpuri"
)

// Composite implements embedius fs.Service delegating to MCP or AFS
// depending on the URI scheme.
type Composite struct {
	mcp *Service
	afs afs.Service
}

// NewComposite constructs a composite fs service.
func NewComposite(mgr *mcpmgr.Manager) *Composite {
	return &Composite{mcp: New(mgr), afs: afs.New()}
}

func (c *Composite) List(ctx context.Context, location string) ([]storage.Object, error) {
	if mcpuri.Is(location) {
		return c.mcp.List(ctx, location)
	}
	return c.afs.List(ctx, location)
}

func (c *Composite) Download(ctx context.Context, object storage.Object) ([]byte, error) {
	if object == nil {
		return nil, nil
	}
	if mcpuri.Is(object.URL()) {
		return c.mcp.Download(ctx, object)
	}
	return c.afs.Download(ctx, object)
}

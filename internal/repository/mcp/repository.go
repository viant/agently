package mcprepo

import (
	"github.com/viant/afs"
	baserepo "github.com/viant/agently/internal/repository/base"
	"github.com/viant/agently/internal/workspace"
	mcpcfg "github.com/viant/fluxor-mcp/mcp/config"
)

// Repository manages MCP client option configs stored in $AGENTLY_ROOT/mcp.
type Repository struct {
	*baserepo.Repository[mcpcfg.MCPClient]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[mcpcfg.MCPClient](fs, workspace.KindMCP)}
}

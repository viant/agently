package mcp

import (
	"github.com/viant/afs"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/internal/workspace/repository/base"
)

// Repository manages MCP client option configs stored in $AGENTLY_WORKSPACE/mcp.
type Repository struct {
	*baserepo.Repository[mcpcfg.MCPClient]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[mcpcfg.MCPClient](fs, workspace.KindMCP)}
}

package mcprepo

import (
	"github.com/viant/afs"
	baserepo "github.com/viant/agently/internal/repository/base"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/mcp"
)

// Repository manages MCP client option configs stored in $AGENTLY_ROOT/mcp.
type Repository struct {
	*baserepo.Repository[mcp.ClientOptions]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[mcp.ClientOptions](fs, workspace.KindMCP)}
}

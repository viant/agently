package manager

import (
	"context"

	"github.com/viant/afs"
	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcprepo "github.com/viant/agently/internal/workspace/repository/mcp"
)

// RepoProvider loads MCP client options from the Agently workspace repo ($AGENTLY_ROOT/mcp).
type RepoProvider struct {
	repo *mcprepo.Repository
}

func NewRepoProvider() *RepoProvider { return &RepoProvider{repo: mcprepo.New(afs.New())} }

func (p *RepoProvider) Options(ctx context.Context, name string) (*mcpcfg.MCPClient, error) {
	return p.repo.Load(ctx, name)
}

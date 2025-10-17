package modelrepo

import (
	"github.com/viant/afs"
	llmprovider "github.com/viant/agently/genai/embedder/provider"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/internal/workspace/repository/base"
)

// Repository provides CRUD over YAML model configs.
type Repository struct {
	*baserepo.Repository[llmprovider.Config]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[llmprovider.Config](fs, workspace.KindEmbedder)}
}

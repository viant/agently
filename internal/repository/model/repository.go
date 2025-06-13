package modelrepo

import (
	"github.com/viant/afs"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	baserepo "github.com/viant/agently/internal/repository/base"
	"github.com/viant/agently/internal/workspace"
)

// Repository provides CRUD over YAML model configs.
type Repository struct {
	*baserepo.Repository[llmprovider.Config]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[llmprovider.Config](fs, workspace.KindModel)}
}

package workflowrepo

import (
	"github.com/viant/afs"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/internal/workspace/repository/base"
	"github.com/viant/fluxor/model"
)

type Repository struct {
	*baserepo.Repository[model.Workflow]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[model.Workflow](fs, workspace.KindWorkflow)}
}

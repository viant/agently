package agentrepo

import (
	"context"

	"github.com/viant/afs"
	"github.com/viant/agently/internal/workspace/repository/base"

	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/internal/workspace"
)

type Repository struct {
	*baserepo.Repository[agent.Agent]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[agent.Agent](fs, workspace.KindAgent)}
}

// SwitchModel updates modelRef and persists.
func (r *Repository) SwitchModel(ctx context.Context, agentId, modelName string) error {
	ag, err := r.Load(ctx, agentId)
	if err != nil {
		return err
	}
	ag.Model = modelName
	return r.Repository.Save(ctx, agentId, ag)
}

// ResetModel clears modelRef.
func (r *Repository) ResetModel(ctx context.Context, agentId string) error {
	ag, err := r.Load(ctx, agentId)
	if err != nil {
		return err
	}
	ag.Model = ""
	return r.Repository.Save(ctx, agentId, ag)
}

package agentrepo

import (
	"context"
	"github.com/viant/afs"

	"github.com/viant/agently/genai/agent"
	baserepo "github.com/viant/agently/internal/repository/base"
	"github.com/viant/agently/internal/workspace"
)

type Repository struct {
	*baserepo.Repository[agent.Agent]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[agent.Agent](fs, workspace.KindAgent)}
}

// SwitchModel updates modelRef and persists.
func (r *Repository) SwitchModel(ctx context.Context, agentName, modelName string) error {
	ag, err := r.Load(ctx, agentName)
	if err != nil {
		return err
	}
	ag.Model = modelName
	return r.Repository.Save(ctx, agentName, ag)
}

// ResetModel clears modelRef.
func (r *Repository) ResetModel(ctx context.Context, agentName string) error {
	ag, err := r.Load(ctx, agentName)
	if err != nil {
		return err
	}
	ag.Model = ""
	return r.Repository.Save(ctx, agentName, ag)
}

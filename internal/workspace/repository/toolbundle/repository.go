package toolbundle

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/tool/bundle"
	"github.com/viant/agently/internal/workspace"
	baserepo "github.com/viant/agently/internal/workspace/repository/base"
)

// Repository loads global tool bundles from $AGENTLY_WORKSPACE/tools/bundles.
type Repository struct {
	*baserepo.Repository[bundle.Bundle]
}

func New(fs afs.Service) *Repository {
	return &Repository{Repository: baserepo.New[bundle.Bundle](fs, workspace.KindToolBundle)}
}

// LoadAll loads and validates all bundles.
func (r *Repository) LoadAll(ctx context.Context) ([]*bundle.Bundle, error) {
	if r == nil || r.Repository == nil {
		return nil, fmt.Errorf("tool bundle repository not configured")
	}
	names, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)

	byID := map[string]*bundle.Bundle{}
	for _, name := range names {
		b, err := r.Load(ctx, name)
		if err != nil {
			return nil, err
		}
		if b == nil {
			continue
		}
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("invalid tool bundle %q: %w", name, err)
		}
		id := strings.TrimSpace(b.ID)
		if _, exists := byID[id]; exists {
			return nil, fmt.Errorf("duplicate tool bundle id %q", id)
		}
		byID[id] = b
	}
	out := make([]*bundle.Bundle, 0, len(byID))
	for _, b := range byID {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

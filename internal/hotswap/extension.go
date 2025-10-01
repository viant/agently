package hotswap

import (
	"context"
	extfinder "github.com/viant/agently/internal/finder/extension"
	extensionrepo "github.com/viant/agently/internal/repository/extension"
)

// NewExtensionAdaptor wires the extensions repository to the in-memory finder
// so edits in the workspace are reflected live.
func NewExtensionAdaptor(repo *extensionrepo.Repository, finder *extfinder.Finder) Reloadable {
	if repo == nil || finder == nil {
		panic("hotswap: nil extension repo/finder")
	}
	// Provide a small custom Reloadable implementation that delegates to repo.
	return &extensionReloadable{repo: repo, finder: finder}
}

type extensionReloadable struct {
	repo   *extensionrepo.Repository
	finder *extfinder.Finder
}

func (e *extensionReloadable) Reload(ctx context.Context, name string, what Action) error {
	switch what {
	case Delete:
		e.finder.Remove(name)
		return nil
	case AddOrUpdate:
		rec, err := e.repo.Load(ctx, name)
		if err != nil {
			return err
		}
		e.finder.Add(name, rec)
		return nil
	default:
		return nil
	}
}

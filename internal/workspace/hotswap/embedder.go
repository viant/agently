package hotswap

import (
	"context"

	provider "github.com/viant/agently/genai/embedder/provider"
	embedfinder "github.com/viant/agently/internal/finder/embedder"
	embedload "github.com/viant/agently/internal/workspace/loader/embedder"
)

// NewEmbedderAdaptor wires embedder loader + finder for hot-swap.
func NewEmbedderAdaptor(loader *embedload.Service, finder *embedfinder.Finder) Reloadable {
	if loader == nil || finder == nil {
		panic("hotswap: embedder adaptor requires non-nil loader and finder")
	}

	loadFn := func(ctx context.Context, name string) (*provider.Config, error) {
		return loader.Load(ctx, name)
	}

	setFn := func(name string, cfg *provider.Config) { finder.AddConfig(name, cfg) }

	removeFn := finder.Remove

	return NewAdaptor[*provider.Config](loadFn, setFn, removeFn)
}

package hotswap

import (
	"context"

	provider "github.com/viant/agently/genai/llm/provider"
	modelfinder "github.com/viant/agently/internal/finder/model"
	modelload "github.com/viant/agently/internal/loader/model"
)

// NewModelAdaptor wires model config loader with model finder to support
// live reload when YAML files under $WORKSPACE/models/ change.
func NewModelAdaptor(loader *modelload.Service, finder *modelfinder.Finder) Reloadable {
	if loader == nil || finder == nil {
		panic("hotswap: model adaptor requires non-nil loader and finder")
	}

	loadFn := func(ctx context.Context, name string) (*provider.Config, error) {
		return loader.Load(ctx, name)
	}

	setFn := func(name string, cfg *provider.Config) {
		if cfg == nil {
			return
		}

		// Primary key: prefer explicit ID; fallback to incoming name.
		key := cfg.ID
		if key == "" {
			key = name
		}
		finder.AddConfig(key, cfg)

		// Alias: when key and file-name differ make both resolve to the same
		// config so callers using either identifier observe the update.
		if key != name {
			finder.AddConfig(name, cfg)
		}
	}

	removeFn := func(name string) { finder.Remove(name) }

	return NewAdaptor[*provider.Config](loadFn, setFn, removeFn)
}

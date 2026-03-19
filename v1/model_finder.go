package v1

import (
	"context"
	"fmt"
	"sync"

	"github.com/viant/agently-core/genai/llm"
	provider "github.com/viant/agently-core/genai/llm/provider"
)

// modelFinder is a minimal llm.Finder implementation owned by agently-app.
// It intentionally avoids agently-core internal packages while supporting
// runtime model resolution from workspace loader configs.
type modelFinder struct {
	loader  provider.ConfigLoader
	factory *provider.Factory

	mu     sync.RWMutex
	models map[string]llm.Model
}

func newModelFinder(loader provider.ConfigLoader) *modelFinder {
	return &modelFinder{
		loader:  loader,
		factory: provider.New(),
		models:  map[string]llm.Model{},
	}
}

func (f *modelFinder) Find(ctx context.Context, id string) (llm.Model, error) {
	f.mu.RLock()
	if model, ok := f.models[id]; ok && model != nil {
		f.mu.RUnlock()
		return model, nil
	}
	f.mu.RUnlock()

	cfg, err := f.loader.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("model config not found: %s", id)
	}

	model, err := f.factory.CreateModel(ctx, &cfg.Options)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.models[id] = model
	f.mu.Unlock()
	return model, nil
}

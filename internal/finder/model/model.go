package model

import (
	"context"
	"fmt"
	"github.com/viant/agently/genai/llm"
	provider "github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/internal/registry"
	"sync"
)

type Finder struct {
	modelFactory   *provider.Factory
	configRegistry *registry.Registry[*provider.Config]
	configLoader   provider.ConfigLoader
	models         map[string]llm.Model
	mux            sync.RWMutex
}

func (d *Finder) Find(ctx context.Context, id string) (llm.Model, error) {
	d.mux.RLock()
	ret, ok := d.models[id]
	d.mux.RUnlock()
	if ok {
		return ret, nil
	}
	d.mux.Lock()
	defer d.mux.Unlock()
	if ret, ok = d.models[id]; ok {
		return ret, nil
	}
	config, err := d.configRegistry.Lookup(ctx, id)
	if err != nil {
		if d.configLoader != nil {
			config, err = d.configLoader.Load(ctx, id)
		}
		if err != nil {
			return nil, err
		}
	}
	if config == nil {
		return nil, fmt.Errorf("model config not found: %s", id)
	}
	model, err := d.modelFactory.CreateModel(ctx, &config.Options)
	if err != nil {
		return nil, err
	}
	d.models[id] = model
	return model, nil
}

func New(options ...Option) *Finder {
	dao := &Finder{
		modelFactory:   provider.New(),
		configRegistry: registry.New[*provider.Config](),
		models:         map[string]llm.Model{},
	}
	for _, option := range options {
		option(dao)
	}

	return dao
}

package model

import (
	"context"
	"fmt"
	"sync"

	"github.com/viant/agently/genai/llm"
	provider "github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/genai/usage"
	"github.com/viant/agently/internal/matcher"
	"github.com/viant/agently/internal/registry"
)

type Finder struct {
	modelFactory   *provider.Factory
	configRegistry *registry.Registry[*provider.Config]
	configLoader   provider.ConfigLoader
	models         map[string]llm.Model
	mux            sync.RWMutex
}

func (d *Finder) Best(p *llm.ModelPreferences) string {
	return d.Matcher().Best(p)
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

	// Attach context Usage Aggregator as UsageListener when present and when
	// the config does not already define one.
	if agg := usage.FromContext(ctx); agg != nil {
		if config.Options.UsageListener == nil {
			// Pass method value so it conforms to base.UsageListener (function type)
			config.Options.UsageListener = func(model string, u *llm.Usage) {
				agg.OnUsage(model, u)
			}
		}
	}

	model, err := d.modelFactory.CreateModel(ctx, &config.Options)
	if err != nil {
		return nil, err
	}
	d.models[id] = model
	return model, nil
}

// Candidates returns lightweight view used by matcher.
func (d *Finder) Candidates() []matcher.Candidate {
	d.mux.RLock()
	defer d.mux.RUnlock()

	out := make([]matcher.Candidate, 0, len(d.models))
	for id := range d.models {
		cfg, _ := d.configRegistry.Lookup(context.Background(), id)
		if cfg == nil {
			continue
		}
		out = append(out, matcher.Candidate{
			ID:           id,
			Intelligence: cfg.Intelligence,
			Speed:        cfg.Speed,
		})
	}
	return out
}

// Matcher builds a matcher instance from current configs.
func (d *Finder) Matcher() *matcher.Matcher {
	return matcher.New(d.Candidates())
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

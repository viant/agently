package model

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

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
	version        int64
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

// TokenPrices returns per-1k token prices for the specified model ID when
// available in the model configuration. Returns ok=false when no config exists
// or prices are not set.
func (d *Finder) TokenPrices(id string) (in float64, out float64, cached float64, ok bool) {
	if strings.TrimSpace(id) == "" {
		return 0, 0, 0, false
	}
	cfg, err := d.configRegistry.Lookup(context.Background(), id)
	if err != nil || cfg == nil {
		return 0, 0, 0, false
	}
	in = cfg.Options.InputTokenPrice
	out = cfg.Options.OutputTokenPrice
	cached = cfg.Options.CachedTokenPrice
	if in == 0 && out == 0 && cached == 0 {
		return 0, 0, 0, false
	}
	return in, out, cached, true
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

// Remove deletes a model configuration and any instantiated model from the
// finder caches. It bumps the internal version so hot-swap watchers can
// detect the change.
func (d *Finder) Remove(name string) {
	d.mux.Lock()
	if _, ok := d.models[name]; ok {
		delete(d.models, name)
	}
	d.mux.Unlock()

	d.configRegistry.Remove(name)
	atomic.AddInt64(&d.version, 1)
}

// Version returns monotonically increasing value changed on Add/Remove.
func (d *Finder) Version() int64 { return atomic.LoadInt64(&d.version) }

// DropModel removes an already instantiated llm.Model instance but keeps its
// configuration. Next Find() will create a fresh model using the existing
// config. Useful after model implementation reload without deleting YAML.
func (d *Finder) DropModel(name string) {
	d.mux.Lock()
	if _, ok := d.models[name]; ok {
		delete(d.models, name)
		atomic.AddInt64(&d.version, 1)
	}
	d.mux.Unlock()
}

// AddConfig injects or overwrites a model configuration and bumps version.
func (d *Finder) AddConfig(name string, cfg *provider.Config) {
	if cfg == nil || name == "" {
		return
	}
	d.configRegistry.Add(name, cfg)
	// Drop any instantiated model to ensure next Find builds a fresh one.
	d.DropModel(name)
	atomic.AddInt64(&d.version, 1)
}

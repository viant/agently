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

// BestWithFilter selects the best model after reducing candidates using allow.
// When the filter excludes all candidates, it falls back to the full set.
func (d *Finder) BestWithFilter(p *llm.ModelPreferences, allow func(id string) bool) string {
	cands := d.Candidates()
	if allow != nil {
		filtered := make([]matcher.Candidate, 0, len(cands))
		for _, c := range cands {
			if allow(c.ID) {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			return matcher.New(filtered).Best(p)
		}
	}
	return matcher.New(cands).Best(p)
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
	// 1) Try direct lookup by config id (registry key)
	if cfg, err := d.configRegistry.Lookup(context.Background(), id); err == nil && cfg != nil {
		in = cfg.Options.InputTokenPrice
		out = cfg.Options.OutputTokenPrice
		cached = cfg.Options.CachedTokenPrice
		if in != 0 || out != 0 || cached != 0 {
			return in, out, cached, true
		}
	}
	// 2) Fallback: scan all configs and match either config.ID or provider model name
	if all, err := d.configRegistry.List(context.Background()); err == nil {
		for _, cfg := range all {
			if cfg == nil {
				continue
			}
			if strings.EqualFold(cfg.ID, id) || strings.EqualFold(strings.TrimSpace(cfg.Options.Model), strings.TrimSpace(id)) {
				in = cfg.Options.InputTokenPrice
				out = cfg.Options.OutputTokenPrice
				cached = cfg.Options.CachedTokenPrice
				if in != 0 || out != 0 || cached != 0 {
					return in, out, cached, true
				}
			}
		}
	}
	return 0, 0, 0, false
}

// Candidates returns lightweight view used by matcher.
func (d *Finder) Candidates() []matcher.Candidate {
	// Build candidates from current model configs. We intentionally base
	// this on configuration registry instead of instantiated models so that
	// matching can consider all known models, even those not yet used.
	configs, err := d.configRegistry.List(context.Background())
	if err != nil {
		return nil
	}
	out := make([]matcher.Candidate, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		cost := 0.0
		if cfg.Options.InputTokenPrice > 0 || cfg.Options.OutputTokenPrice > 0 {
			cost = cfg.Options.InputTokenPrice + cfg.Options.OutputTokenPrice
		}
		out = append(out, matcher.Candidate{
			ID:           cfg.ID,
			Intelligence: cfg.Intelligence,
			Speed:        cfg.Speed,
			Cost:         cost,
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

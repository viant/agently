package oauthfinder

import (
	"context"

	"github.com/viant/agently/genai/oauth2"
	"github.com/viant/agently/internal/registry"
)

// Finder provides in-memory lookup for OAuth2 configurations.
type Finder struct {
	reg *registry.Registry[*oauth2.Config]
}

// New constructs an empty Finder instance.
func New() *Finder {
	return &Finder{reg: registry.New[*oauth2.Config]()}
}

// AddConfig stores (or overwrites) cfg keyed by cfg.ID.
func (f *Finder) AddConfig(id string, cfg *oauth2.Config) {
	if f == nil || cfg == nil || id == "" {
		return
	}
	f.reg.Add(id, cfg)
}

// Find returns config by id.
func (f *Finder) Find(ctx context.Context, id string) (*oauth2.Config, error) {
	return f.reg.Lookup(ctx, id)
}

// List returns all configs.
func (f *Finder) List(ctx context.Context) ([]*oauth2.Config, error) {
	return f.reg.List(ctx)
}

// Remove deletes config from registry.
func (f *Finder) Remove(id string) { f.reg.Remove(id) }

// Version returns monotonically increasing counter from underlying registry.
func (f *Finder) Version() int64 { return f.reg.Version() }

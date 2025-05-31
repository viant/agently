package embedder

import provider "github.com/viant/agently/genai/embedder/provider"

// Option mutates Finder during construction.
type Option func(*Finder)

// WithInitial registers a list of configuration objects available for
// lookup by ID.
func WithInitial(config ...*provider.Config) Option {
	return func(d *Finder) {
		for _, cfg := range config {
			d.configRegistry.Add(cfg.ID, cfg)
		}
	}
}

func WithConfigLoader(loader provider.ConfigLoader) Option {
	return func(d *Finder) {
		d.loader = loader
	}
}

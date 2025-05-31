package agent

import "github.com/viant/agently/genai/agent"

// Option mutates Finder during construction.
type Option func(*Finder)

func WithLoader(l agent.Loader) Option { return func(d *Finder) { d.loader = l } }

func WithInitial(agents ...*agent.Agent) Option {
	return func(d *Finder) {
		for _, a := range agents {
			d.Add(a.ID, a)
		}
	}
}

package agent

import "context"

// Finder is an interface for finding Agents by their ID.
type Finder interface {
	Find(ctx context.Context, id string) (*Agent, error)
}

package agent

import (
	"context"
)

// Loader exposes operations required by higher-level services on top of the
// concrete Loader implementation.  The interface is intentionally minimal to keep
// package dependencies low – additional Loader methods should be added only when
// they are genuinely used by an upstream layer.
type Loader interface {
	// Add stores an in-memory representation of an Agent so it becomes
	// available for subsequent queries.
	Add(name string, agent *Agent)

	//Load retrieves an Agent by its name. If the Agent does not exist, it
	Load(ctx context.Context, name string) (*Agent, error)
}

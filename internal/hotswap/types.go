package hotswap

import "context"

// Action represents the type of change detected on the workspace resource.
type Action int

const (
	// AddOrUpdate signals that a file was created or modified.
	AddOrUpdate Action = iota
	// Delete signals that a file was removed or renamed so it is no longer available.
	Delete
)

// Reloadable is implemented by any registry/loader pair that can accept
// hot-swap notifications originating from the workspace file watcher.
//
// The HotSwapManager invokes Reload whenever a YAML file inside the
// workspace is added, changed or deleted.
//
// name – base file name without extension (e.g. "chat" for chatter.yaml)
// what – kind of change (AddOrUpdate/Delete).
//
// Implementations are expected to be thread-safe.
type Reloadable interface {
	Reload(ctx context.Context, name string, what Action) error
}

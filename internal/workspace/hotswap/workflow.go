package hotswap

import (
	"context"
	"path/filepath"
	"strings"
)

// WorkflowAdaptor previously pushed workspace YAML changes into a runtime
// through user-supplied callbacks. In decoupled mode this remains a generic
// hot‑reload helper and does not depend on any orchestration engine.
//
// loadRaw   – returns raw YAML bytes for a workflow name (without .yaml).
// absPathFn – resolves the canonical location string that Runtime uses as key
//
//	(usually workspace/kind/<name>.yaml).
//
// refresh   – invalidates cached copy (called on Delete when upsert is nil).
// upsert    – optional; when non-nil it is used on AddOrUpdate so that runtime
//
//	is updated immediately without next-load round-trip.
func NewWorkflowAdaptor(
	loadRaw func(ctx context.Context, name string) ([]byte, error),
	absPathFn func(name string) string,
	refresh func(location string) error,
	upsert func(location string, data []byte) error,
) Reloadable {

	if loadRaw == nil || absPathFn == nil || refresh == nil {
		panic("workflow adaptor: loadRaw, absPathFn and refresh must be provided")
	}

	// fallback when runtime lacks upsert support
	if upsert == nil {
		upsert = func(location string, _ []byte) error { return refresh(location) }
	}

	return &workflowAdaptor{
		loadRaw: loadRaw,
		absPath: absPathFn,
		refresh: refresh,
		upsert:  upsert,
	}
}

type workflowAdaptor struct {
	loadRaw func(ctx context.Context, name string) ([]byte, error)
	absPath func(name string) string
	refresh func(location string) error
	upsert  func(location string, data []byte) error
}

func (w *workflowAdaptor) Reload(ctx context.Context, name string, what Action) error {
	location := w.absPath(name)

	switch what {
	case Delete:
		return w.refresh(location)

	case AddOrUpdate:
		data, err := w.loadRaw(ctx, name)
		if err != nil {
			return err
		}
		return w.upsert(location, data)
	}
	return nil
}

// Helper resolving canonical path mirroring repository default logic.
func ResolveWorkflowPath(name string) string {
	if filepath.Ext(name) == "" {
		name += ".yaml"
	}
	if strings.Contains(name, "/") || strings.Contains(name, "://") {
		return filepath.ToSlash(name)
	}
	// Flat workspace layout: workflows/<name>.yaml
	return filepath.ToSlash(filepath.Join("workflows", name))
}

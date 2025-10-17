package hotswap

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// change is an internal representation of a single file-system update that
// the manager needs to dispatch to the appropriate registry.
type change struct {
	kind   string // workspace kind, e.g. "agents"
	name   string // basename without extension, e.g. "chat"
	action Action
}

// Manager orchestrates hot-swap live reload across multiple workspace kinds.
// Callers should create one instance per executor and invoke Start/Stop in
// sync with the executor lifecycle.
type Manager struct {
	root     string
	debounce time.Duration

	watcher *fsnotify.Watcher
	regs    map[string]Reloadable // key: workspace.Kind*

	changes chan change
	ctx     context.Context
	cancel  context.CancelFunc

	mu sync.Mutex // guards regs
}

// NewManager builds a Manager watching workspace root. Debounce specifies the
// minimum interval between two events for the same file before the latter is
// forwarded; use 0 for no debouncing.
func NewManager(root string, debounce time.Duration) (*Manager, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		root:     filepath.Clean(root),
		debounce: debounce,
		watcher:  w,
		regs:     map[string]Reloadable{},
		changes:  make(chan change, 64),
		ctx:      ctx,
		cancel:   cancel,
	}
	return m, nil
}

// Register attaches a Reloadable registry to a workspace kind such as
// workspace.KindAgent. Must be called before Start.
func (m *Manager) Register(kind string, r Reloadable) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.regs[kind] = r
}

// Start begins watching the workspace and dispatching change events. It
// spawns goroutines and returns immediately. Calling Start twice is a no-op.
func (m *Manager) Start() error {
	// Recursively add directories under root so we receive events for nested
	// files (e.g. agents/chat).
	if err := filepath.WalkDir(m.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return m.watcher.Add(path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("hotswap: failed to register watcher: %w", err)
	}

	// Goroutine: translate fs events → change → m.changes
	go m.loopWatch()
	// Goroutine: dispatch changes to registries serially
	go m.loopDispatch()
	return nil
}

// Stop shuts down the manager and underlying watcher. Safe for repeated use.
func (m *Manager) Stop() {
	m.cancel()
	_ = m.watcher.Close()
}

// helper ----------------------------------------------------------

func (m *Manager) loopWatch() {
	debounceMap := map[string]time.Time{}
	for {
		select {
		case <-m.ctx.Done():
			return

		case ev, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if !isYAML(ev.Name) {
				continue
			}

			action, ok := translateOp(ev.Op)
			if !ok {
				continue
			}

			relPath, err := filepath.Rel(m.root, ev.Name)
			if err != nil {
				continue // outside root – ignore
			}
			parts := splitPath(relPath)
			if len(parts) < 2 { // need <kind>/<name>.yaml minimum
				continue
			}

			kind := parts[0]
			base := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))

			// Debounce to collapse rapid sequences on the same file.
			if m.debounce > 0 {
				key := kind + "/" + base
				if ts, exists := debounceMap[key]; exists {
					if time.Since(ts) < m.debounce {
						// skip duplicate event
						continue
					}
				}
				debounceMap[key] = time.Now()
			}

			m.changes <- change{kind: kind, name: base, action: action}
		}
	}
}

func splitPath(p string) []string {
	p = filepath.ToSlash(p)
	return strings.Split(p, "/")
}

func (m *Manager) loopDispatch() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case ch := <-m.changes:
			m.mu.Lock()
			reg := m.regs[ch.kind]
			m.mu.Unlock()
			if reg == nil {
				continue // no registry for this kind
			}
			_ = reg.Reload(m.ctx, ch.name, ch.action) // errors ignored here; caller can wrap reg to log errors
		}
	}
}

func translateOp(op fsnotify.Op) (Action, bool) {
	switch {
	case op&fsnotify.Create == fsnotify.Create,
		op&fsnotify.Write == fsnotify.Write,
		op&fsnotify.Rename == fsnotify.Rename:
		return AddOrUpdate, true
	case op&fsnotify.Remove == fsnotify.Remove:
		return Delete, true
	default:
		return AddOrUpdate, false
	}
}

func isYAML(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

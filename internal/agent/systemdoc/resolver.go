package systemdoc

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	agmodel "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/internal/workspace"
)

// Resolver caches system resource prefixes per agent so callers can classify
// document URIs without reloading agent definitions on every lookup.
type Resolver struct {
	finder agmodel.Finder

	mu    sync.RWMutex
	cache map[string][]string
}

// NewResolver builds a Resolver backed by the provided agent finder.
func NewResolver(f agmodel.Finder) *Resolver {
	return &Resolver{
		finder: f,
		cache:  map[string][]string{},
	}
}

// IsSystem reports whether uri belongs to a system-scoped resource for agentID.
func (r *Resolver) IsSystem(ctx context.Context, agentID, uri string) bool {
	return Matches(r.Prefixes(ctx, agentID), uri)
}

// Prefixes returns the cached system prefixes for an agent, loading them when needed.
func (r *Resolver) Prefixes(ctx context.Context, agentID string) []string {
	if r == nil {
		return nil
	}
	id := strings.TrimSpace(agentID)
	if id == "" {
		return nil
	}
	r.mu.RLock()
	if prefixes, ok := r.cache[id]; ok {
		r.mu.RUnlock()
		return prefixes
	}
	r.mu.RUnlock()

	if r.finder == nil {
		return nil
	}
	agent, err := r.finder.Find(ctx, id)
	if err != nil || agent == nil {
		return nil
	}
	prefixes := Prefixes(agent)
	r.mu.Lock()
	r.cache[id] = prefixes
	r.mu.Unlock()
	return prefixes
}

// Clear evicts cached prefixes for an agent. Useful in tests.
func (r *Resolver) Clear(agentID string) {
	if r == nil {
		return
	}
	id := strings.TrimSpace(agentID)
	if id == "" {
		return
	}
	r.mu.Lock()
	delete(r.cache, id)
	r.mu.Unlock()
}

// Prefixes extracts normalized system resource prefixes from the agent definition.
func Prefixes(agent *agmodel.Agent) []string {
	if agent == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(uri string) {
		if strings.TrimSpace(uri) == "" {
			return
		}
		norm := Normalize(uri)
		if norm == "" {
			return
		}
		if _, ok := seen[norm]; ok {
			return
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	for _, res := range agent.Resources {
		if res == nil || !strings.EqualFold(strings.TrimSpace(res.Role), "system") {
			continue
		}
		add(res.URI)
	}
	for _, knowledge := range agent.SystemKnowledge {
		if knowledge == nil {
			continue
		}
		add(knowledge.URL)
	}
	return out
}

// Matches reports whether uri is under any of the provided system prefixes.
func Matches(prefixes []string, uri string) bool {
	if len(prefixes) == 0 {
		return false
	}
	normalized := Normalize(uri)
	if normalized == "" {
		return false
	}
	for _, prefix := range prefixes {
		p := strings.TrimRight(strings.TrimSpace(prefix), "/")
		if p == "" {
			continue
		}
		if normalized == p {
			return true
		}
		if strings.HasPrefix(normalized, p+"/") {
			return true
		}
	}
	return false
}

// Normalize converts various URI/path forms into a comparable workspace/file URI.
func Normalize(uri string) string {
	u := strings.TrimSpace(uri)
	if u == "" {
		return ""
	}
	u = strings.ReplaceAll(u, "\\", "/")
	lower := strings.ToLower(u)
	switch {
	case strings.HasPrefix(lower, "workspace://"):
		return normalizeWorkspaceURI(u)
	case strings.HasPrefix(lower, "file://"):
		return normalizeAbsolutePath(filepath.FromSlash(fileURLToPath(u)))
	case filepath.IsAbs(u):
		return normalizeAbsolutePath(filepath.FromSlash(u))
	case strings.Contains(u, "://"):
		return strings.TrimRight(u, "/")
	default:
		abs := filepath.Join(workspace.Root(), u)
		return normalizeAbsolutePath(abs)
	}
}

func normalizeWorkspaceURI(uri string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(uri), "/")
	if trimmed == "" {
		return ""
	}
	rest := trimmed[len("workspace://"):]
	if idx := strings.Index(rest, "/"); idx != -1 {
		host := rest[:idx]
		path := rest[idx+1:]
		if strings.EqualFold(host, "localhost") {
			rest = path
		} else {
			rest = host + "/" + path
		}
	} else if strings.EqualFold(rest, "localhost") {
		rest = ""
	}
	rest = strings.TrimLeft(rest, "/")
	if rest == "" {
		return "workspace://localhost"
	}
	return "workspace://localhost/" + rest
}

func normalizeAbsolutePath(path string) string {
	cleaned := filepath.Clean(path)
	if rel, ok := trimWorkspacePrefix(cleaned); ok {
		rel = filepath.ToSlash(rel)
		if rel == "" || rel == "." {
			return "workspace://localhost"
		}
		return "workspace://localhost/" + strings.TrimLeft(rel, "/")
	}
	return "file://" + filepath.ToSlash(cleaned)
}

func trimWorkspacePrefix(path string) (string, bool) {
	root := filepath.Clean(workspace.Root())
	if equalPath(path, root) {
		return "", true
	}
	prefix := root + string(filepath.Separator)
	if hasPathPrefix(path, prefix) {
		return path[len(prefix):], true
	}
	return "", false
}

func equalPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func hasPathPrefix(path, prefix string) bool {
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
	}
	return strings.HasPrefix(path, prefix)
}

func fileURLToPath(u string) string {
	if strings.TrimSpace(u) == "" {
		return ""
	}
	rest := u
	if idx := strings.Index(rest, "://"); idx != -1 {
		rest = rest[idx+3:]
	}
	rest = strings.TrimPrefix(rest, "localhost")
	rest = strings.TrimPrefix(rest, "localhost/")
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
	}
	if len(rest) >= 3 && rest[0] == '/' && rest[2] == ':' {
		rest = rest[1:]
	}
	return rest
}

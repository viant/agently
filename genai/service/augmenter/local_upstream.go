package augmenter

import (
	"context"
	"database/sql"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/viant/afs/url"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/datly/view"
)

type localRoot struct {
	ID          string
	URI         string
	UpstreamRef string
	matchKey    string
}

type localRootsKey struct{}

// WithLocalRoots attaches per-request local roots for upstream resolution.
func WithLocalRoots(ctx context.Context, roots []LocalRoot) context.Context {
	if ctx == nil || len(roots) == 0 {
		return ctx
	}
	return context.WithValue(ctx, localRootsKey{}, roots)
}

func localRootsFromContext(ctx context.Context) []LocalRoot {
	if ctx == nil {
		return nil
	}
	if val := ctx.Value(localRootsKey{}); val != nil {
		if roots, ok := val.([]LocalRoot); ok {
			return roots
		}
	}
	return nil
}

func normalizeLocalRoots(roots []LocalRoot) []localRoot {
	if len(roots) == 0 {
		return nil
	}
	out := make([]localRoot, 0, len(roots))
	for _, root := range roots {
		uri := strings.TrimSpace(root.URI)
		if uri == "" {
			continue
		}
		matchKey := normalizeLocalMatchKey(uri)
		if matchKey == "" {
			continue
		}
		out = append(out, localRoot{
			ID:          strings.TrimSpace(root.ID),
			URI:         uri,
			UpstreamRef: strings.TrimSpace(root.UpstreamRef),
			matchKey:    matchKey,
		})
	}
	return out
}

func normalizeLocalUpstreams(upstreams []LocalUpstream) map[string]LocalUpstream {
	if len(upstreams) == 0 {
		return nil
	}
	out := make(map[string]LocalUpstream, len(upstreams))
	for _, upstream := range upstreams {
		name := strings.TrimSpace(upstream.Name)
		if name == "" {
			continue
		}
		upstream.Name = name
		out[name] = upstream
	}
	return out
}

func normalizeLocalMatchKey(uri string) string {
	loc := normalizeLocalLocation(uri)
	return normalizeLocalPathKey(loc)
}

func normalizeLocalLocation(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "workspace://") {
		return workspaceToFile(v)
	}
	if strings.HasPrefix(lower, "file://") {
		return strings.TrimRight(v, "/")
	}
	if strings.Contains(v, "://") {
		return strings.TrimRight(v, "/")
	}
	if filepath.IsAbs(v) || isWindowsAbsPath(v) {
		return url.Normalize(v, "file")
	}
	rel := strings.TrimPrefix(v, "/")
	return workspaceToFile(url.Join("workspace://localhost/", rel))
}

func normalizeLocalPathKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		rootPath := pathpkg.Clean("/" + strings.TrimLeft(url.Path(value), "/"))
		return rootPath
	}
	return filepath.ToSlash(filepath.Clean(value))
}

func isUnderLocalPath(path, root string) bool {
	path = strings.TrimSuffix(strings.TrimSpace(path), "/")
	root = strings.TrimSuffix(strings.TrimSpace(root), "/")
	if path == "" || root == "" {
		return false
	}
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+"/")
}

func workspaceToFile(ws string) string {
	base := url.Normalize(workspace.Root(), "file")
	rel := strings.TrimPrefix(ws, "workspace://")
	rel = strings.TrimPrefix(rel, "localhost/")
	return url.Join(strings.TrimRight(base, "/")+"/", rel)
}

func isLocalUpstreamEnabled(upstream *LocalUpstream) bool {
	if upstream == nil {
		return false
	}
	if upstream.Enabled == nil {
		return true
	}
	return *upstream.Enabled
}

func (s *Service) resolveLocalUpstream(ctx context.Context, location string) (*localRoot, *LocalUpstream, bool) {
	if s == nil {
		return nil, nil, false
	}
	locKey := normalizeLocalMatchKey(location)
	if locKey == "" {
		return nil, nil, false
	}
	roots := s.localRoots
	if extra := normalizeLocalRoots(localRootsFromContext(ctx)); len(extra) > 0 {
		merged := make([]localRoot, 0, len(roots)+len(extra))
		merged = append(merged, roots...)
		merged = append(merged, extra...)
		roots = merged
	}
	if len(roots) == 0 {
		return nil, nil, false
	}
	var best *localRoot
	for i := range roots {
		root := &roots[i]
		if root.UpstreamRef == "" || root.matchKey == "" {
			continue
		}
		if !isUnderLocalPath(locKey, root.matchKey) {
			continue
		}
		if best == nil || len(root.matchKey) > len(best.matchKey) {
			best = root
		}
	}
	if best == nil {
		return nil, nil, false
	}
	if s.localUpstreams == nil {
		return best, nil, false
	}
	upstream, ok := s.localUpstreams[strings.TrimSpace(best.UpstreamRef)]
	if !ok {
		return best, nil, false
	}
	return best, &upstream, true
}

func (s *Service) upstreamDBLocal(ctx context.Context, upstream *LocalUpstream) (*sql.DB, error) {
	conn := view.NewConnector("embedius_upstream", upstream.Driver, upstream.DSN)
	db, err := conn.DB()
	if err != nil {
		return nil, err
	}
	if err := s.pingDB(ctx, db); err != nil {
		return nil, err
	}
	return db, nil
}

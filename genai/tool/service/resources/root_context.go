package resources

import (
	"context"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/viant/afs/url"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/workspace"
)

type rootContext struct {
	alias  string
	wsRoot string
	base   string
}

func (s *Service) newRootContext(ctx context.Context, rootURI, rootID string, allowed []string) (*rootContext, error) {
	root := strings.TrimSpace(rootURI)
	if root == "" && strings.TrimSpace(rootID) != "" {
		var err error
		root, err = s.resolveRootID(ctx, rootID)
		if err != nil {
			return nil, err
		}
	}
	if root == "" {
		return nil, fmt.Errorf("root or rootId is required")
	}
	wsRoot, _, err := s.normalizeUserRoot(ctx, root)
	if err != nil {
		return nil, err
	}
	if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
		return nil, fmt.Errorf("root not allowed: %s", root)
	}
	base := wsRoot
	if strings.HasPrefix(wsRoot, "workspace://") {
		base = workspaceToFile(wsRoot)
	}
	return &rootContext{
		alias:  root,
		wsRoot: wsRoot,
		base:   base,
	}, nil
}

func (rc *rootContext) ResolvePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return rc.base, nil
	}
	return joinBaseWithPath(rc.wsRoot, rc.base, strings.TrimSpace(p), rc.alias)
}

func (rc *rootContext) Base() string {
	return rc.base
}

func (rc *rootContext) Workspace() string {
	return rc.wsRoot
}

func (s *Service) normalizeFullURI(ctx context.Context, uri string, allowed []string) (string, error) {
	wsRoot, _, err := s.normalizeUserRoot(ctx, uri)
	if err != nil {
		return "", err
	}
	if len(allowed) > 0 && !isAllowedWorkspace(wsRoot, allowed) {
		return "", fmt.Errorf("resource not allowed: %s", uri)
	}
	if strings.HasPrefix(wsRoot, "workspace://") {
		return workspaceToFile(wsRoot), nil
	}
	return wsRoot, nil
}

func isAbsLikePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return true
	}
	if strings.HasPrefix(p, "file://") {
		return true
	}
	if mcpuri.Is(p) {
		return true
	}
	return false
}

func fileURLToPath(u string) string {
	u = strings.TrimSpace(u)
	if !strings.HasPrefix(u, "file://") {
		return u
	}
	rest := strings.TrimPrefix(u, "file://")
	rest = strings.TrimPrefix(rest, "localhost")
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return rest
}

func isUnderRootPath(path, root string) bool {
	path = strings.TrimSpace(path)
	root = strings.TrimSpace(root)
	if path == "" || root == "" {
		return false
	}
	if strings.Contains(root, "://") {
		rootPath := pathpkg.Clean("/" + strings.TrimLeft(url.Path(root), "/"))
		pathPath := pathpkg.Clean("/" + strings.TrimLeft(url.Path(path), "/"))
		if rootPath == "/" {
			return true
		}
		if pathPath == rootPath {
			return true
		}
		if !strings.HasSuffix(rootPath, "/") {
			rootPath += "/"
		}
		return strings.HasPrefix(pathPath, rootPath)
	}
	pathFS := filepath.Clean(path)
	rootFS := filepath.Clean(root)
	if rootFS == string(os.PathSeparator) {
		return true
	}
	if pathFS == rootFS {
		return true
	}
	if !strings.HasSuffix(rootFS, string(os.PathSeparator)) {
		rootFS += string(os.PathSeparator)
	}
	return strings.HasPrefix(pathFS, rootFS)
}

func joinBaseWithPath(wsRoot, base, p, rootAlias string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return base, nil
	}
	if isAbsLikePath(p) {
		if !mcpuri.Is(wsRoot) {
			rootBase := base
			if strings.HasPrefix(rootBase, "file://") {
				rootBase = fileURLToPath(rootBase)
			}
			pathBase := p
			if strings.HasPrefix(pathBase, "file://") {
				pathBase = fileURLToPath(pathBase)
			}
			if !isUnderRootPath(pathBase, rootBase) {
				return "", fmt.Errorf("path %s is outside root %s", p, rootAlias)
			}
		}
		return p, nil
	}
	return url.Join(base, strings.TrimPrefix(p, "/")), nil
}

func relativePath(rootURI, fullURI string) string {
	root := strings.TrimSuffix(strings.TrimSpace(rootURI), "/")
	uri := strings.TrimSpace(fullURI)
	if root == "" || uri == "" {
		return ""
	}
	if !strings.HasPrefix(uri, root) {
		return uri
	}
	rel := strings.TrimPrefix(uri[len(root):], "/")
	return rel
}

func workspaceToFile(ws string) string {
	base := url.Normalize(workspace.Root(), "file")
	rel := strings.TrimPrefix(ws, "workspace://")
	rel = strings.TrimPrefix(rel, "localhost/")
	return url.Join(strings.TrimRight(base, "/")+"/", rel)
}

func fileToWorkspace(file string) string {
	base := workspace.Root()
	file = strings.Replace(file, base, "", 1)
	return "workspace://localhost" + url.Path(file)
}

func cleanFileURL(u string) string {
	if !strings.HasPrefix(u, "file://") {
		return u
	}
	rest := strings.TrimPrefix(u, "file://localhost")
	rest = strings.TrimPrefix(rest, "file://")
	cleaned := "/" + strings.TrimLeft(rest, "/")
	cleaned = pathpkg.Clean(cleaned)
	return "file://localhost" + cleaned
}

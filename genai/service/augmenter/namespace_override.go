package augmenter

import (
	"context"
	"strings"

	mcpcfg "github.com/viant/agently/internal/mcp/config"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/embedius/document"
	"github.com/viant/embedius/indexer"
	"github.com/viant/embedius/indexer/cache"
	"github.com/viant/embedius/schema"
)

type namespaceOverrideIndexer struct {
	base    indexer.Indexer
	resolve func(ctx context.Context, uri string) (string, bool, error)
}

func (n *namespaceOverrideIndexer) Index(ctx context.Context, URI string, cache *cache.Map[string, document.Entry]) ([]schema.Document, []string, error) {
	return n.base.Index(ctx, URI, cache)
}

func (n *namespaceOverrideIndexer) Namespace(ctx context.Context, URI string) (string, error) {
	if n.resolve != nil {
		if ns, ok, err := n.resolve(ctx, URI); err != nil {
			return "", err
		} else if ok && strings.TrimSpace(ns) != "" {
			return ns, nil
		}
	}
	return n.base.Namespace(ctx, URI)
}

func newNamespaceOverrideIndexer(base indexer.Indexer, resolve func(ctx context.Context, uri string) (string, bool, error)) indexer.Indexer {
	if base == nil || resolve == nil {
		return base
	}
	return &namespaceOverrideIndexer{base: base, resolve: resolve}
}

// resolveMCPRootID returns the MCP root ID for the given location when available.
func (s *Service) resolveMCPRootID(ctx context.Context, location string) (string, bool, error) {
	if s == nil || s.mcpMgr == nil || !mcpuri.Is(location) {
		return "", false, nil
	}
	locServer, locPaths := mcpuri.CompareParts(location)
	if strings.TrimSpace(locServer) == "" || len(locPaths) == 0 {
		return "", false, nil
	}
	opts, err := s.mcpMgr.Options(ctx, locServer)
	if err != nil || opts == nil {
		return "", false, err
	}
	roots := mcpcfg.ResourceRoots(opts.Metadata)
	if len(roots) == 0 {
		return "", false, nil
	}
	for _, root := range roots {
		rootServer, rootPaths := mcpuri.CompareParts(root.URI)
		if strings.TrimSpace(rootServer) == "" || len(rootPaths) == 0 {
			continue
		}
		if rootServer != locServer {
			continue
		}
		matched := false
		for _, lp := range locPaths {
			for _, rp := range rootPaths {
				if lp == rp || strings.HasPrefix(lp, rp+"/") {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			id := strings.TrimSpace(root.ID)
			if id == "" {
				return "", false, nil
			}
			return id, true, nil
		}
	}
	return "", false, nil
}

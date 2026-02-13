package mcpfs

import (
	"context"
	"encoding/base64"
	"fmt"
	neturl "net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/viant/afs/storage"
	"github.com/viant/agently/genai/memory"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	mcpschema "github.com/viant/mcp-protocol/schema"
)

// Service implements embedius fs.Service for MCP resources.
// It lists and downloads resources via a per-conversation MCP client manager.
type Service struct {
	mgr              *mcpmgr.Manager
	snapshotResolver SnapshotResolver
	manifestResolver SnapshotManifestResolver
	snapshotRoot     string
	snapshotMu       sync.Mutex
	snapshots        map[string]*snapshotCache
	snapInFlight     map[string]*snapshotWait
	snapRefresh      map[string]struct{}
	snapSizeMu       sync.RWMutex
	snapSizes        map[string]int64
}

type snapshotWait struct {
	done chan struct{}
	err  error
}

// Option configures an MCP fs service.
type Option func(*Service)

// WithSnapshotResolver instructs the MCP fs to prefer snapshot reads when available.
func WithSnapshotResolver(resolver SnapshotResolver) Option {
	return func(s *Service) {
		s.snapshotResolver = resolver
	}
}

// WithSnapshotManifestResolver instructs the MCP fs to use snapshot MD5 manifests when enabled.
func WithSnapshotManifestResolver(resolver SnapshotManifestResolver) Option {
	return func(s *Service) {
		s.manifestResolver = resolver
	}
}

// WithSnapshotCacheRoot overrides the snapshot cache root template.
func WithSnapshotCacheRoot(template string) Option {
	return func(s *Service) {
		s.snapshotRoot = strings.TrimSpace(template)
	}
}

// New returns an MCP-backed fs service.
func New(mgr *mcpmgr.Manager, opts ...Option) *Service {
	s := &Service{mgr: mgr}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// SetSnapshotResolver updates the snapshot resolver for an existing MCP fs service.
func (s *Service) SetSnapshotResolver(resolver SnapshotResolver) {
	if s == nil {
		return
	}
	s.snapshotResolver = resolver
}

// SetSnapshotManifestResolver updates the snapshot manifest resolver for an existing MCP fs service.
func (s *Service) SetSnapshotManifestResolver(resolver SnapshotManifestResolver) {
	if s == nil {
		return
	}
	s.manifestResolver = resolver
}

// List returns MCP resources under the given location prefix.
// Accepts formats: mcp://server/path or mcp:server:/path
func (s *Service) List(ctx context.Context, location string) ([]storage.Object, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	if snapURI, rootURI, ok := s.resolveSnapshot(location); ok {
		return s.listSnapshot(ctx, location, snapURI, rootURI)
	}
	fmt.Printf("mcpfs: list start location=%q\n", location)
	server, prefix := mcpuri.Parse(location)
	if strings.TrimSpace(server) == "" {
		return nil, fmt.Errorf("mcpfs: invalid location: %s", location)
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			convID = tm.ConversationID
		}
	}
	if strings.TrimSpace(convID) == "" {
		return nil, fmt.Errorf("mcpfs: missing conversation id in context")
	}
	cli, err := s.mgr.Get(ctx, convID, server)
	if err != nil {
		return nil, fmt.Errorf("mcpfs: get client: %w", err)
	}
	ctx = s.mgr.WithAuthTokenContext(ctx, server)
	if _, err := cli.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcpfs: init: %w", err)
	}

	var out []storage.Object
	var cursor *string
	matched := 0
	for {
		res, err := cli.ListResources(ctx, cursor)
		if err != nil {
			return nil, fmt.Errorf("mcpfs: list resources: %w", err)
		}
		for _, r := range res.Resources {
			if prefix != "" && !matchesMCPPrefix(r.Uri, prefix) {
				continue
			}
			if r.Size != nil && *r.Size > 0 {
				s.recordSnapshotSize(r.Uri, int64(*r.Size))
			}
			out = append(out, newObject(server, r))
			matched++
		}
		if res.NextCursor == nil || strings.TrimSpace(*res.NextCursor) == "" {
			break
		}
		cursor = res.NextCursor
	}
	if prefix != "" && matched == 0 {
		fmt.Printf("mcpfs: warning: no resources matched prefix %q on server %q\n", prefix, server)
	}
	fmt.Printf("mcpfs: list done location=%q matched=%d total=%d\n", location, matched, len(out))
	return out, nil
}

// SnapshotUpToDate reports whether a cached snapshot matches the remote size.
func (s *Service) SnapshotUpToDate(ctx context.Context, location string) (bool, error) {
	if s == nil || s.mgr == nil {
		return false, nil
	}
	snapURI, _, ok := s.resolveSnapshot(location)
	if !ok {
		return false, nil
	}
	s.snapshotMu.Lock()
	cache := s.snapshots[snapURI]
	s.snapshotMu.Unlock()
	if cache == nil {
		return false, nil
	}
	cachedSize := cache.size
	if cachedSize <= 0 {
		if fi, err := os.Stat(cache.path); err == nil && fi.Mode().IsRegular() {
			cachedSize = fi.Size()
			cache.size = cachedSize
		}
	}
	if cachedSize <= 0 {
		return false, nil
	}
	remoteSize, ok := s.snapshotSize(snapURI)
	if !ok || remoteSize <= 0 {
		if size, ok, err := s.fetchSnapshotSize(ctx, snapURI); err != nil {
			return false, err
		} else if ok {
			remoteSize = size
		}
	}
	if remoteSize <= 0 {
		return true, nil
	}
	return remoteSize == cachedSize, nil
}

func (s *Service) recordSnapshotSize(uri string, size int64) {
	if s == nil || size <= 0 {
		return
	}
	if !strings.HasSuffix(strings.ToLower(uri), "/_snapshot.zip") && !strings.HasSuffix(strings.ToLower(uri), "_snapshot.zip") {
		return
	}
	s.snapSizeMu.Lock()
	if s.snapSizes == nil {
		s.snapSizes = map[string]int64{}
	}
	s.snapSizes[normalizeMCPURL(uri)] = size
	s.snapSizeMu.Unlock()
}

func (s *Service) snapshotSize(uri string) (int64, bool) {
	s.snapSizeMu.RLock()
	defer s.snapSizeMu.RUnlock()
	if s.snapSizes == nil {
		return 0, false
	}
	size, ok := s.snapSizes[normalizeMCPURL(uri)]
	return size, ok
}

func (s *Service) fetchSnapshotSize(ctx context.Context, snapURI string) (int64, bool, error) {
	server, _ := mcpuri.Parse(snapURI)
	if strings.TrimSpace(server) == "" {
		return 0, false, nil
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			convID = tm.ConversationID
		}
	}
	if strings.TrimSpace(convID) == "" {
		return 0, false, nil
	}
	cli, err := s.mgr.Get(ctx, convID, server)
	if err != nil {
		return 0, false, err
	}
	ctx = s.mgr.WithAuthTokenContext(ctx, server)
	if _, err := cli.Initialize(ctx); err != nil {
		return 0, false, err
	}
	var cursor *string
	target := normalizeMCPURL(snapURI)
	for {
		res, err := cli.ListResources(ctx, cursor)
		if err != nil {
			return 0, false, err
		}
		for _, r := range res.Resources {
			if normalizeMCPURL(r.Uri) != target {
				continue
			}
			if r.Size != nil && *r.Size > 0 {
				size := int64(*r.Size)
				s.recordSnapshotSize(r.Uri, size)
				return size, true, nil
			}
			return 0, false, nil
		}
		if res.NextCursor == nil || strings.TrimSpace(*res.NextCursor) == "" {
			break
		}
		cursor = res.NextCursor
	}
	return 0, false, nil
}

// Download reads the MCP resource contents for the given object.
func (s *Service) Download(ctx context.Context, object storage.Object) ([]byte, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	if object == nil {
		return nil, nil
	}
	mcpURL := object.URL()
	server, uri := mcpuri.Parse(mcpURL)
	if strings.TrimSpace(server) == "" || strings.TrimSpace(uri) == "" {
		return nil, fmt.Errorf("mcpfs: invalid mcp url: %s", mcpURL)
	}
	if snapURI, rootURI, ok := s.resolveSnapshot(mcpURL); ok {
		// Log snapshot requests at root-level only to avoid per-file noise.
		if normalizeMCPURL(mcpURL) == normalizeMCPURL(snapURI) {
			cache, err := s.ensureSnapshot(ctx, snapURI)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(cache.path)
			return data, err
		}
		cache, err := s.ensureSnapshot(ctx, snapURI)
		if err != nil {
			return nil, err
		}
		if so, ok := object.(*snapshotObject); ok {
			return s.downloadSnapshotFile(cache, so.archivePath, s.resolveManifest(mcpURL))
		}
		return s.downloadSnapshotByURI(cache, rootURI, mcpURL)
	}
	data, err := s.downloadRaw(ctx, mcpURL)
	if err == nil {
		// no-op
	}
	return data, err
}

// DownloadDirect bypasses snapshot resolution and fetches the resource directly.
func (s *Service) DownloadDirect(ctx context.Context, object storage.Object) ([]byte, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	if object == nil {
		return nil, nil
	}
	mcpURL := object.URL()
	server, uri := mcpuri.Parse(mcpURL)
	if strings.TrimSpace(server) == "" || strings.TrimSpace(uri) == "" {
		return nil, fmt.Errorf("mcpfs: invalid mcp url: %s", mcpURL)
	}
	data, err := s.downloadRaw(ctx, mcpURL)
	if err == nil {
		// no-op
	}
	return data, err
}

func matchesMCPPrefix(uri, prefix string) bool {
	if prefix == "" {
		return true
	}
	if strings.HasPrefix(uri, prefix) {
		return true
	}
	if u, err := neturl.Parse(uri); err == nil {
		if strings.HasPrefix(u.Path, prefix) {
			return true
		}
		if u.Host != "" {
			combined := "/" + u.Host + u.Path
			return strings.HasPrefix(combined, prefix)
		}
	}
	return false
}

func (s *Service) downloadRaw(ctx context.Context, mcpURL string) ([]byte, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	server, uri := mcpuri.Parse(mcpURL)
	if strings.TrimSpace(server) == "" || strings.TrimSpace(uri) == "" {
		return nil, fmt.Errorf("mcpfs: invalid mcp url: %s", mcpURL)
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			convID = tm.ConversationID
		}
	}
	if strings.TrimSpace(convID) == "" {
		return nil, fmt.Errorf("mcpfs: missing conversation id in context")
	}
	cli, err := s.mgr.Get(ctx, convID, server)
	if err != nil {
		return nil, fmt.Errorf("mcpfs: get client: %w", err)
	}
	ctx = s.mgr.WithAuthTokenContext(ctx, server)
	if _, err := cli.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcpfs: init: %w", err)
	}

	res, err := cli.ReadResource(ctx, &mcpschema.ReadResourceRequestParams{Uri: uri})
	if err != nil {
		return nil, fmt.Errorf("mcpfs: read resource: %w", err)
	}
	var data []byte
	for _, c := range res.Contents {
		if c.Text != "" {
			data = append(data, []byte(c.Text)...)
			continue
		}
		if c.Blob != "" {
			if dec, err := base64.StdEncoding.DecodeString(c.Blob); err == nil {
				data = append(data, dec...)
			}
		}
	}
	return data, nil
}

// -------------------- helpers --------------------

// object implements storage.Object over an MCP resource entry.
type object struct {
	server string
	uri    string
	name   string
	size   int64
	isDir  bool
	url    string
	mod    time.Time
	src    interface{}
}

func newObject(server string, r mcpschema.Resource) storage.Object {
	size := int64(0)
	if r.Size != nil {
		size = int64(*r.Size)
	}
	name := r.Name
	if name == "" {
		name = path.Base(r.Uri)
	}
	return &object{
		server: server,
		uri:    r.Uri,
		name:   name,
		size:   size,
		url:    mcpuri.Canonical(server, r.Uri),
		mod:    time.Now(),
		isDir:  false,
		src:    r,
	}
}

// NewObjectFromURI builds a minimal storage.Object for a given mcp URL.
// It is useful for direct downloads when a full Resource descriptor is not available.
func NewObjectFromURI(mcpURL string) storage.Object {
	server, uri := mcpuri.Parse(mcpURL)
	name := path.Base(uri)
	return &object{
		server: server,
		uri:    uri,
		name:   name,
		size:   0,
		url:    mcpURL,
		mod:    time.Now(),
		isDir:  false,
		src:    nil,
	}
}

// ---- os.FileInfo ----
func (o *object) Name() string       { return o.name }
func (o *object) Size() int64        { return o.size }
func (o *object) Mode() os.FileMode  { return 0o444 }
func (o *object) ModTime() time.Time { return o.mod }
func (o *object) IsDir() bool        { return o.isDir }
func (o *object) Sys() interface{}   { return nil }

// ---- storage.Object ----
func (o *object) URL() string          { return o.url }
func (o *object) Wrap(src interface{}) { o.src = src }
func (o *object) Unwrap(dst interface{}) error {
	// Best-effort assign when types match
	if dst == nil || o.src == nil {
		return nil
	}
	if p, ok := dst.(*mcpschema.Resource); ok {
		if v, ok := o.src.(mcpschema.Resource); ok {
			*p = v
			return nil
		}
	}
	return nil
}

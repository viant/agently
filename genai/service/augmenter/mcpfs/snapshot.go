package mcpfs

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/viant/afs/storage"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
)

// SnapshotResolver maps a location to an MCP snapshot URI and its root.
type SnapshotResolver func(location string) (snapshotURI, rootURI string, ok bool)

// SnapshotManifestResolver reports whether a location should use snapshot MD5 manifests.
type SnapshotManifestResolver func(location string) bool

type snapshotCache struct {
	path        string
	stripPrefix string
	size        int64
	manifest    map[string]manifestEntry
	manifestMu  sync.Mutex
}

type snapshotObject struct {
	object
	snapshotURI string
	rootURI     string
	archivePath string
	md5         string
}

type manifestEntry struct {
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	MD5     string `json:"md5"`
}

func (s *Service) resolveSnapshot(location string) (string, string, bool) {
	if s == nil || s.snapshotResolver == nil {
		return "", "", false
	}
	return s.snapshotResolver(location)
}

func (s *Service) resolveManifest(location string) bool {
	if s == nil || s.manifestResolver == nil {
		return false
	}
	return s.manifestResolver(location)
}

func (s *Service) ensureSnapshot(ctx context.Context, snapURI string) (*snapshotCache, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	s.snapshotMu.Lock()
	if s.snapshots == nil {
		s.snapshots = map[string]*snapshotCache{}
	}
	if entry, ok := s.snapshots[snapURI]; ok && entry != nil {
		s.snapshotMu.Unlock()
		s.maybeRefreshSnapshot(ctx, snapURI)
		return entry, nil
	}
	if s.snapInFlight == nil {
		s.snapInFlight = map[string]*snapshotWait{}
	}
	if wait, ok := s.snapInFlight[snapURI]; ok && wait != nil {
		s.snapshotMu.Unlock()
		<-wait.done
		if wait.err != nil {
			return nil, wait.err
		}
		s.snapshotMu.Lock()
		entry := s.snapshots[snapURI]
		s.snapshotMu.Unlock()
		if entry == nil {
			return nil, fmt.Errorf("mcpfs: snapshot missing after wait: %s", snapURI)
		}
		return entry, nil
	}
	wait := &snapshotWait{done: make(chan struct{})}
	s.snapInFlight[snapURI] = wait
	s.snapshotMu.Unlock()

	sharedPath := s.snapshotCachePath(ctx, snapURI)
	if sharedPath != "" {
		if fi, err := os.Stat(sharedPath); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
			stripPrefix, err := detectStripPrefix(sharedPath)
			if err != nil {
				return nil, err
			}
			entry := &snapshotCache{path: sharedPath, stripPrefix: stripPrefix, size: fi.Size()}
			s.snapshotMu.Lock()
			if s.snapshots == nil {
				s.snapshots = map[string]*snapshotCache{}
			}
			s.snapshots[snapURI] = entry
			if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
				w.err = nil
				close(w.done)
				delete(s.snapInFlight, snapURI)
			}
			s.snapshotMu.Unlock()
			return entry, nil
		}
	}

	data, err := s.downloadRaw(ctx, snapURI)
	if err != nil {
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	if sharedPath == "" {
		sharedPath = s.snapshotCachePath(ctx, snapURI)
	}
	if sharedPath == "" {
		return nil, fmt.Errorf("mcpfs: snapshot cache path empty")
	}
	if err := os.MkdirAll(filepath.Dir(sharedPath), 0o755); err != nil {
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	tmp := sharedPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	if err := os.Rename(tmp, sharedPath); err != nil {
		_ = os.Remove(tmp)
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	stripPrefix, err := detectStripPrefix(sharedPath)
	if err != nil {
		s.snapshotMu.Lock()
		if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
			w.err = err
			close(w.done)
			delete(s.snapInFlight, snapURI)
		}
		s.snapshotMu.Unlock()
		return nil, err
	}
	entry := &snapshotCache{path: sharedPath, stripPrefix: stripPrefix, size: int64(len(data))}
	s.snapshotMu.Lock()
	if s.snapshots == nil {
		s.snapshots = map[string]*snapshotCache{}
	}
	s.snapshots[snapURI] = entry
	if w, ok := s.snapInFlight[snapURI]; ok && w != nil {
		w.err = nil
		close(w.done)
		delete(s.snapInFlight, snapURI)
	}
	s.snapshotMu.Unlock()
	return entry, nil
}

func (s *Service) maybeRefreshSnapshot(ctx context.Context, snapURI string) {
	if s == nil || s.mgr == nil {
		return
	}
	upToDate, err := s.SnapshotUpToDate(ctx, snapURI)
	if err != nil || upToDate {
		return
	}
	s.snapshotMu.Lock()
	if s.snapInFlight != nil {
		if _, ok := s.snapInFlight[snapURI]; ok {
			s.snapshotMu.Unlock()
			return
		}
	}
	if s.snapRefresh == nil {
		s.snapRefresh = map[string]struct{}{}
	}
	if _, ok := s.snapRefresh[snapURI]; ok {
		s.snapshotMu.Unlock()
		return
	}
	s.snapRefresh[snapURI] = struct{}{}
	s.snapshotMu.Unlock()

	go func() {
		defer func() {
			s.snapshotMu.Lock()
			delete(s.snapRefresh, snapURI)
			s.snapshotMu.Unlock()
		}()
		bgCtx := context.WithoutCancel(ctx)
		_ = s.refreshSnapshot(bgCtx, snapURI)
	}()
}

func (s *Service) refreshSnapshot(ctx context.Context, snapURI string) error {
	if s == nil || s.mgr == nil {
		return fmt.Errorf("mcpfs: manager not configured")
	}
	data, err := s.downloadRaw(ctx, snapURI)
	if err != nil {
		return err
	}
	sharedPath := s.snapshotCachePath(ctx, snapURI)
	if sharedPath == "" {
		return fmt.Errorf("mcpfs: snapshot cache path empty")
	}
	if err := os.MkdirAll(filepath.Dir(sharedPath), 0o755); err != nil {
		return err
	}
	tmp := sharedPath + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, sharedPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	stripPrefix, err := detectStripPrefix(sharedPath)
	if err != nil {
		return err
	}
	entry := &snapshotCache{path: sharedPath, stripPrefix: stripPrefix, size: int64(len(data))}
	s.snapshotMu.Lock()
	if s.snapshots == nil {
		s.snapshots = map[string]*snapshotCache{}
	}
	s.snapshots[snapURI] = entry
	s.snapshotMu.Unlock()
	return nil
}

func (s *Service) snapshotCachePath(ctx context.Context, snapURI string) string {
	root := s.snapshotCacheRoot(ctx)
	if strings.TrimSpace(root) == "" {
		return ""
	}
	norm := normalizeMCPURL(snapURI)
	sum := sha1.Sum([]byte(norm))
	name := hex.EncodeToString(sum[:]) + ".zip"
	return filepath.Join(root, name)
}

func detectStripPrefix(zipPath string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	common := ""
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(f.Name, "/")
		if name == "" {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 || parts[0] == "" {
			return "", nil
		}
		if common == "" {
			common = parts[0]
			continue
		}
		if common != parts[0] {
			return "", nil
		}
	}
	if common == "" {
		return "", nil
	}
	return common + "/", nil
}

func (s *Service) listSnapshot(ctx context.Context, location, snapURI, rootURI string) ([]storage.Object, error) {
	cache, err := s.ensureSnapshot(ctx, snapURI)
	if err != nil {
		return nil, err
	}
	if s.resolveManifest(location) && cache.manifest == nil {
		if manifest, err := ensureManifest(cache); err == nil {
			cache.manifest = manifest
		}
	}
	manifest := cache.manifest
	rootURI = normalizeMCPURL(rootURI)
	if rootURI == "" {
		return nil, fmt.Errorf("mcpfs: invalid snapshot root: %s", rootURI)
	}
	normLoc := normalizeMCPURL(location)
	relPrefix := ""
	if normLoc != "" && strings.HasPrefix(normLoc, rootURI) {
		relPrefix = strings.TrimPrefix(normLoc, rootURI)
		relPrefix = strings.TrimPrefix(relPrefix, "/")
	}

	reader, err := zip.OpenReader(cache.path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	server, rootPath := mcpuri.Parse(rootURI)
	rootPath = strings.TrimRight(rootPath, "/")
	if rootPath == "" {
		rootPath = "/"
	}
	var out []storage.Object
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(f.Name, cache.stripPrefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		if relPrefix != "" && rel != relPrefix && !strings.HasPrefix(rel, relPrefix+"/") {
			continue
		}
		fullPath := mcpuri.JoinResourcePath(rootPath, rel)
		md5hex := ""
		if manifest != nil {
			if entry, ok := manifest[f.Name]; ok {
				mod := f.Modified.UTC().Format(time.RFC3339Nano)
				if entry.Size == int64(f.UncompressedSize64) && entry.ModTime == mod && entry.MD5 != "" {
					md5hex = entry.MD5
				}
			}
		}
		obj := &snapshotObject{
			object: object{
				server: server,
				uri:    fullPath,
				name:   path.Base(rel),
				size:   int64(f.UncompressedSize64),
				url:    mcpuri.Canonical(server, fullPath),
				mod:    f.Modified,
				isDir:  false,
			},
			snapshotURI: snapURI,
			rootURI:     rootURI,
			archivePath: f.Name,
			md5:         md5hex,
		}
		out = append(out, obj)
	}
	return out, nil
}

func (s *Service) downloadSnapshotByURI(cache *snapshotCache, rootURI, mcpURL string) ([]byte, error) {
	zipPath, err := snapshotZipPath(rootURI, mcpURL, cache.stripPrefix)
	if err != nil {
		return nil, err
	}
	return s.downloadSnapshotFile(cache, zipPath, s.resolveManifest(mcpURL))
}

func (s *Service) downloadSnapshotFile(cache *snapshotCache, zipPath string, useManifest bool) ([]byte, error) {
	if cache == nil {
		return nil, fmt.Errorf("mcpfs: snapshot not available")
	}
	if useManifest && cache.manifest == nil {
		if manifest, err := ensureManifest(cache); err == nil {
			cache.manifest = manifest
		}
	}
	reader, err := zip.OpenReader(cache.path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	for _, f := range reader.File {
		if f.Name != zipPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}
		if useManifest && cache.manifest != nil {
			mod := f.Modified.UTC().Format(time.RFC3339Nano)
			md5hex := fmt.Sprintf("%x", md5.Sum(data))
			_ = updateManifest(cache, zipPath, manifestEntry{
				Size:    int64(f.UncompressedSize64),
				ModTime: mod,
				MD5:     md5hex,
			})
		}
		return data, nil
	}
	return nil, fmt.Errorf("mcpfs: snapshot missing entry: %s", zipPath)
}

func (o *snapshotObject) MD5() string {
	return o.md5
}

func manifestPath(cache *snapshotCache) string {
	if cache == nil || cache.path == "" {
		return ""
	}
	return strings.TrimSuffix(cache.path, ".zip") + ".json"
}

func ensureManifest(cache *snapshotCache) (map[string]manifestEntry, error) {
	if cache == nil || cache.path == "" {
		return nil, nil
	}
	if manifest, err := loadManifest(cache); err == nil && manifest != nil {
		return manifest, nil
	}
	return buildManifest(cache)
}

func buildManifest(cache *snapshotCache) (map[string]manifestEntry, error) {
	if cache == nil || cache.path == "" {
		return nil, nil
	}
	reader, err := zip.OpenReader(cache.path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	manifest := map[string]manifestEntry{}
	total := len(reader.File)
	scanned := 0
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		hasher := md5.New()
		if _, err := io.Copy(hasher, rc); err != nil {
			_ = rc.Close()
			return nil, err
		}
		_ = rc.Close()
		manifest[f.Name] = manifestEntry{
			Size:    int64(f.UncompressedSize64),
			ModTime: f.Modified.UTC().Format(time.RFC3339Nano),
			MD5:     hex.EncodeToString(hasher.Sum(nil)),
		}
		scanned++
		if scanned%5000 == 0 {
			fmt.Printf("mcpfs: manifest progress path=%q scanned=%d total=%d\n", cache.path, scanned, total)
		}
	}
	cache.manifestMu.Lock()
	cache.manifest = manifest
	cache.manifestMu.Unlock()
	if err := persistManifest(cache, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func loadManifest(cache *snapshotCache) (map[string]manifestEntry, error) {
	if cache == nil {
		return nil, nil
	}
	cache.manifestMu.Lock()
	defer cache.manifestMu.Unlock()
	if cache.manifest != nil {
		return cache.manifest, nil
	}
	path := manifestPath(cache)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	manifest := map[string]manifestEntry{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	cache.manifest = manifest
	return manifest, nil
}

func updateManifest(cache *snapshotCache, zipPath string, entry manifestEntry) error {
	if cache == nil || zipPath == "" {
		return nil
	}
	cache.manifestMu.Lock()
	defer cache.manifestMu.Unlock()
	if cache.manifest == nil {
		cache.manifest = map[string]manifestEntry{}
	}
	cache.manifest[zipPath] = entry
	return persistManifest(cache, cache.manifest)
}

func persistManifest(cache *snapshotCache, manifest map[string]manifestEntry) error {
	path := manifestPath(cache)
	if path == "" {
		return nil
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func snapshotZipPath(rootURI, mcpURL, stripPrefix string) (string, error) {
	root := normalizeMCPURL(rootURI)
	target := normalizeMCPURL(mcpURL)
	if root == "" || target == "" {
		return "", fmt.Errorf("mcpfs: invalid snapshot mapping")
	}
	if !strings.HasPrefix(target, root) {
		return "", fmt.Errorf("mcpfs: resource not under snapshot root")
	}
	rel := strings.TrimPrefix(target, root)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", fmt.Errorf("mcpfs: snapshot resource missing path")
	}
	if stripPrefix != "" {
		return stripPrefix + rel, nil
	}
	return rel, nil
}

func normalizeMCPURL(location string) string {
	return mcpuri.NormalizeForCompare(location)
}

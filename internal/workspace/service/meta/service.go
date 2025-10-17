package meta

import (
	"context"
	"encoding/json"
	"path"
	"path/filepath"
	"strings"

	"github.com/viant/afs"
	"gopkg.in/yaml.v3"
)

// Service provides minimal meta loading and listing with a base directory.
type Service struct {
	fs   afs.Service
	base string
}

// New constructs a meta Service with the given filesystem and base directory/URL.
func New(fs afs.Service, base string) *Service { return &Service{fs: fs, base: base} }

// resolve joins base with a relative path, otherwise returns the path as-is.
func (s *Service) resolve(p string) string {
	if p == "" {
		return s.base
	}
	if strings.Contains(p, "://") || filepath.IsAbs(p) {
		return p
	}
	if strings.TrimSpace(s.base) == "" {
		return p
	}
	// When base is a URL, prefer URL-style join to avoid OS path quirks.
	if strings.Contains(s.base, "://") {
		base := strings.TrimRight(s.base, "/")
		rel := strings.TrimLeft(p, "/")
		return base + "/" + rel
	}
	return filepath.Join(s.base, p)
}

// Load reads URL and unmarshals into v. Supports *yaml.Node or a struct pointer.
func (s *Service) Load(ctx context.Context, URL string, v interface{}) error {
	URL = s.resolve(URL)
	data, err := s.fs.DownloadWithURL(ctx, URL)
	if err != nil {
		return err
	}
	switch out := v.(type) {
	case *yaml.Node:
		return yaml.Unmarshal(data, out)
	default:
		// Choose by extension; default to YAML
		switch strings.ToLower(path.Ext(URL)) {
		case ".json":
			return json.Unmarshal(data, v)
		default:
			return yaml.Unmarshal(data, v)
		}
	}
}

// List returns YAML candidates under a directory or the file itself when URL points to a file.
func (s *Service) List(ctx context.Context, URL string) ([]string, error) {
	URL = s.resolve(URL)
	if ext := strings.ToLower(path.Ext(URL)); ext == ".yaml" || ext == ".yml" {
		return []string{URL}, nil
	}
	objs, err := s.fs.List(ctx, URL)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, o := range objs {
		if o.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(o.Name()))
		if ext == ".yaml" || ext == ".yml" {
			out = append(out, s.resolve(filepath.Join(URL, filepath.Base(o.Name()))))
		}
	}
	return out, nil
}

// Exists checks if the resolved URL exists.
func (s *Service) Exists(ctx context.Context, URL string) (bool, error) {
	return s.fs.Exists(ctx, s.resolve(URL))
}

// GetURL returns the resolved absolute URL/path for a possibly relative path.
func (s *Service) GetURL(p string) string { return s.resolve(p) }

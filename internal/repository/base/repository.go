package baserepo

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/fluxor/service/meta"
	"gopkg.in/yaml.v3"
)

// Repository generic CRUD for YAML/JSON resources stored under
// $AGENTLY_ROOT/<kind>/.
type Repository[T any] struct {
	fs   afs.Service
	meta *meta.Service
	dir  string
}

// New constructs a repository for a specific workspace kind (e.g. "models").
func New[T any](fs afs.Service, kind string) *Repository[T] {
	dir := workspace.Path(kind)
	return &Repository[T]{fs: fs, meta: meta.New(fs, dir), dir: dir}
}

// filename resolves name to absolute path with .yaml default extension.
func (r *Repository[T]) filename(name string) string {
	// Ensure we end with .yaml when extension missing.
	if filepath.Ext(name) == "" {
		name += ".yaml"
	}

	// First attempt the flat layout: <dir>/<name>.yaml
	flat := filepath.Join(r.dir, name)

	// Fast path – assume flat layout exists (no FS call).
	if ok, _ := r.fs.Exists(context.TODO(), flat); ok {
		return flat
	}

	// Fallback to historical nested layout: <dir>/<name>/<name>.yaml
	base := strings.TrimSuffix(name, ".yaml")
	nested := filepath.Join(r.dir, base, name)
	return nested
}

// List basenames (without extension).
func (r *Repository[T]) List(ctx context.Context) ([]string, error) {
	objs, err := r.fs.List(ctx, r.dir)
	if err != nil {
		return nil, err
	}
	var res []string

	for _, o := range objs {
		if o.IsDir() {
			// Handle possible nested layout <dir>/<name>/<name>.yaml>
			dirName := filepath.Base(o.Name())
			nested := filepath.Join(r.dir, dirName, dirName+".yaml")
			if ok, _ := r.fs.Exists(ctx, nested); ok {
				res = append(res, dirName)
			}
			continue
		}
		base := filepath.Base(o.Name())
		res = append(res, strings.TrimSuffix(base, filepath.Ext(base)))
	}
	return res, nil
}

// GetRaw downloads raw bytes.
func (r *Repository[T]) GetRaw(ctx context.Context, name string) ([]byte, error) {
	return r.fs.DownloadWithURL(ctx, r.filename(name))
}

// Load unmarshals YAML/JSON into *T.
func (r *Repository[T]) Load(ctx context.Context, name string) (*T, error) {
	var v T
	if err := r.meta.Load(ctx, r.filename(name), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Save (Add/overwrite) marshals struct to YAML.
func (r *Repository[T]) Save(ctx context.Context, name string, obj *T) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	return r.Add(ctx, name, data)
}

// Add uploads raw data.
func (r *Repository[T]) Add(ctx context.Context, name string, data []byte) error {
	return r.fs.Upload(ctx, r.filename(name), file.DefaultFileOsMode, bytes.NewReader(data))
}

// Delete removes file.
func (r *Repository[T]) Delete(ctx context.Context, name string) error {
	return r.fs.Delete(ctx, r.filename(name))
}

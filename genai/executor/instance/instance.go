package instance

// Package instance provides a process-wide singleton wrapper around
// *executor.Service*.  It allows multiple entry-points (CLI, HTTP, gRPC, …) to
// share the same fully-initialised executor without import-ing each other.

import (
	"bytes"
	"context"
	_ "embed"
	"path/filepath"
	"sync"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/internal/workspace"
	"gopkg.in/yaml.v3"
)

//go:embed config/config.yaml
var defaultYAML []byte

var (
	once      sync.Once
	createErr error
	svc       *executor.Service
)

// Init builds the singleton using the supplied executor options.  cfgPath may
// point to a YAML/JSON document describing executor.Config – when non-empty it
// is loaded and passed through executor.WithConfig().  Calling Init more than
// once is safe; only the first call creates the instance, subsequent calls
// return the original error (if any).
func Init(ctx context.Context, cfgPath string, opts ...executor.Option) error {
	once.Do(func() {
		// determine config location – default to workspace root when caller
		// did not override.
		if cfgPath == "" {
			cfgPath = filepath.Join(workspace.Root(), "config.yaml")
		}

		createErr = ensureConfig(ctx, cfgPath, &opts)

		var err error
		svc, err = executor.New(ctx, opts...)
		if err != nil {
			createErr = err
			return
		}
		svc.Start(ctx)
	})
	return createErr
}

func ensureConfig(ctx context.Context, cfgPath string, opts *[]executor.Option) error {
	var cfg *executor.Config
	if cfgPath != "" {
		var err error
		cfg, err = loadOrCreateConfig(ctx, cfgPath)
		if err != nil {
			return err
		}
		*opts = append(*opts, executor.WithConfig(cfg))
	}
	return nil
}

// --------------------------------------------------------------------

// loadOrCreateConfig tries to load a config from path; if file is missing it
// creates a sensible default, writes it to disk, and returns it.
func loadOrCreateConfig(ctx context.Context, path string) (*executor.Config, error) {
	fs := afs.New()
	// 1. File already exists – load and return.
	if ok, _ := fs.Exists(ctx, path); ok {
		return loadConfig(ctx, path)
	}

	// 2. Requested location equals the standard workspace config – let the
	//    workspace bootstrap create defaults, then load.
	if path == filepath.Join(workspace.Root(), "config.yaml") {
		workspace.EnsureDefault(fs)
		return loadConfig(ctx, path)
	}

	// 3. Caller pointed to a *custom* location outside the workspace root –
	//    fall back to the embedded template exactly as before.
	var cfg executor.Config
	if err := yaml.Unmarshal(defaultYAML, &cfg); err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	_ = fs.Create(ctx, dir, 0755, true) // create missing directories
	if err := fs.Upload(ctx, path, 0644, bytes.NewReader(defaultYAML)); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Get returns the singleton (may be nil when Init has not been called or
// failed).
func Get() *executor.Service { return svc }

// Shutdown gracefully terminates the singleton runtime (if created).
func Shutdown(ctx context.Context) {
	if svc != nil {
		svc.Shutdown(ctx)
	}
}

// --------------------------------------------------------------------

func loadConfig(ctx context.Context, url string) (*executor.Config, error) {
	fs := afs.New()
	data, err := fs.DownloadWithURL(ctx, url)
	if err != nil {
		return nil, err
	}
	var cfg executor.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

package workspace

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/viant/afs"
)

const (
	// envKey is the environment variable used to override the default root.
	envKey = "AGENTLY_ROOT"

	// defaultRoot is used when the env variable is not defined.
	defaultRootDir = ".agently"
)

var (
	// cachedRoot holds the resolved, absolute path to the workspace root.
	cachedRoot string
	initOnce   sync.Once
)

// Predefined kinds.  Callers may still supply arbitrary sub-folder names when
// they need custom separation.
const (
	KindAgent    = "agents"
	KindModel    = "models"
	KindEmbedder = "embedders"
	KindMCP      = "mcp"
	KindWorkflow = "workflows"
	KindTool     = "tools"
)

// Root returns the absolute path to the Agently root directory.
// The lookup order is:
//  1. $AGENTLY_ROOT environment variable, if set and non-empty
//  2. $HOME/.agently
//
// The result is cached for the lifetime of the process.
func Root() string {
	if cachedRoot != "" {
		// If a different AGENTLY_ROOT is now set, update the cache so subsequent
		// calls (e.g. in tests) see the correct location.
		if env := os.Getenv(envKey); env != "" && abs(env) != cachedRoot {
			cachedRoot = abs(env)
			_ = os.MkdirAll(cachedRoot, 0755)
			return cachedRoot
		}
		return cachedRoot
	}

	if env := os.Getenv(envKey); env != "" {
		cachedRoot = abs(env)
		_ = os.MkdirAll(cachedRoot, 0755) // ensure root exists
		// Do not auto-populate built-in defaults when a custom workspace root
		// is explicitly supplied via $AGENTLY_ROOT. This gives callers full
		// control over the workspace content (e.g. unit tests expecting an
		// empty repository).
		return cachedRoot
	}

	home, err := os.Getwd()
	if err != nil {
		// Fall back to current working directory on unexpected failure.
		cachedRoot = abs(defaultRootDir)
		return cachedRoot
	}

	cachedRoot = abs(filepath.Join(home, defaultRootDir))
	_ = os.MkdirAll(cachedRoot, 0755) // ensure root exists

	// lazily create default resources once the root directory is ready
	ensureDefaults()
	return cachedRoot
}

// Path returns a sub-path under the root for the given kind (e.g. "agents").
func Path(kind string) string {
	dir := filepath.Join(Root(), kind)
	_ = os.MkdirAll(dir, 0755) // ensure directory exists
	return dir
}

// ensureDefaults writes baseline config/model/agent/workflow files to a fresh
// workspace when they are missing.
func ensureDefaults() {
	initOnce.Do(func() {
		afsSvc := afs.New()
		EnsureDefault(afsSvc)
	})
}

// abs converts p into an absolute, clean path. If an error occurs it returns p
// unchanged – the caller tolerates relative paths.
func abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if absPath, err := filepath.Abs(p); err == nil {
		return absPath
	}
	return filepath.Clean(p)
}

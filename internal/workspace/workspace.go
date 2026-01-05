package workspace

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/viant/afs"
)

const (
	// envKey is the environment variable used to override the default workspace root.
	envKey = "AGENTLY_WORKSPACE"

	// defaultRoot is used when the env variable is not defined.
	defaultRootDir = ".agently"
)

var (
	// cachedRoot holds the resolved, absolute path to the workspace root.
	cachedRoot string
	// defaultsMu guards defaultsByRoot so default bootstrapping runs once per root.
	defaultsMu     sync.Mutex
	defaultsByRoot = map[string]bool{}
)

// Predefined kinds.  Callers may still supply arbitrary sub-folder names when
// they need custom separation.
const (
	KindAgent      = "agents"
	KindModel      = "models"
	KindEmbedder   = "embedders"
	KindMCP        = "mcp"
	KindWorkflow   = "workflows"
	KindTool       = "tools"
	KindToolBundle = "tools/bundles"
	KindToolHints  = "tools/hints"
	KindOAuth      = "oauth"
	KindFeeds      = "feeds"
	KindA2A        = "a2a"
)

// Root returns the absolute path to the Agently workspace directory.
// The lookup order is:
//  1. $AGENTLY_WORKSPACE environment variable, if set and non-empty
//  2. $HOME/.agently
//
// The result is cached for the lifetime of the process.
func Root() string {
	if cachedRoot != "" {
		// If a different AGENTLY_WORKSPACE is now set, update the cache so subsequent
		// calls (e.g. in tests) see the correct location.
		if env := os.Getenv(envKey); env != "" && abs(env) != cachedRoot {
			cachedRoot = abs(env)
			_ = os.MkdirAll(cachedRoot, 0755)
			ensureDefaults(cachedRoot)
			return cachedRoot
		}
		return cachedRoot
	}

	if env := os.Getenv(envKey); env != "" {
		cachedRoot = abs(env)
		_ = os.MkdirAll(cachedRoot, 0755) // ensure root exists
		ensureDefaults(cachedRoot)
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
	ensureDefaults(cachedRoot)
	return cachedRoot
}

// Path returns a sub-path under the root for the given kind (e.g. "agents").
func Path(kind string) string {
	dir := filepath.Join(Root(), kind)
	_ = os.MkdirAll(dir, 0755) // ensure directory exists
	return dir
}

// ensureDefaults writes baseline config/model/agent/workflow files to a workspace
// when they are missing.
//
// It runs at most once per root. Set `AGENTLY_WORKSPACE_NO_DEFAULTS=1` to disable
// default bootstrapping for a given process (useful for unit tests).
func ensureDefaults(root string) {
	if os.Getenv("AGENTLY_WORKSPACE_NO_DEFAULTS") != "" {
		return
	}
	root = abs(root)
	if root == "" {
		return
	}
	defaultsMu.Lock()
	if defaultsByRoot[root] {
		defaultsMu.Unlock()
		return
	}
	defaultsByRoot[root] = true
	defaultsMu.Unlock()

	afsSvc := afs.New()
	EnsureDefault(afsSvc)
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

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	// cachedRuntime holds the resolved runtime root override when set.
	cachedRuntime string
	// cachedState holds the resolved state root override when set.
	cachedState string
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

// RuntimeRoot returns the runtime root path. It defaults to the workspace root
// unless overridden via AGENTLY_RUNTIME_ROOT or SetRuntimeRoot.
func RuntimeRoot() string {
	if cachedRuntime != "" {
		return cachedRuntime
	}
	if env := os.Getenv("AGENTLY_RUNTIME_ROOT"); strings.TrimSpace(env) != "" {
		cachedRuntime = abs(resolveTemplate(env, false))
		_ = os.MkdirAll(cachedRuntime, 0755)
		return cachedRuntime
	}
	cachedRuntime = Root()
	return cachedRuntime
}

// StateRoot returns the state root path. It defaults to RuntimeRoot()/state unless overridden.
func StateRoot() string {
	if cachedState != "" {
		return cachedState
	}
	if env := os.Getenv("AGENTLY_STATE_PATH"); strings.TrimSpace(env) != "" {
		cachedState = abs(resolveTemplate(env, true))
		_ = os.MkdirAll(cachedState, 0755)
		return cachedState
	}
	cachedState = filepath.Join(RuntimeRoot(), "state")
	_ = os.MkdirAll(cachedState, 0755)
	return cachedState
}

// SetRuntimeRoot overrides the runtime root path for this process.
func SetRuntimeRoot(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	cachedRuntime = abs(resolveTemplate(path, false))
	_ = os.MkdirAll(cachedRuntime, 0755)
	// reset derived state root so it can be recomputed
	cachedState = ""
}

// SetStateRoot overrides the state root path for this process.
func SetStateRoot(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	cachedState = abs(resolveTemplate(path, true))
	_ = os.MkdirAll(cachedState, 0755)
}

// ResolvePathTemplate expands supported macros in a path template.
// Supported macros: ${workspaceRoot}, ${runtimeRoot}.
func ResolvePathTemplate(value string) string {
	return strings.TrimSpace(resolveTemplate(value, true))
}

func resolveTemplate(value string, includeRuntime bool) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return v
	}
	v = strings.ReplaceAll(v, "${workspaceRoot}", Root())
	if includeRuntime {
		v = strings.ReplaceAll(v, "${runtimeRoot}", RuntimeRoot())
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		v = strings.ReplaceAll(v, "${home}", home)
	}
	v = expandUserHome(v)
	return v
}

func expandUserHome(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return v
	}
	if strings.HasPrefix(trimmed, "~/") || trimmed == "~" {
		return filepath.Join(home, strings.TrimPrefix(trimmed, "~"))
	}
	if strings.HasPrefix(trimmed, "file://") {
		prefix := "file://localhost"
		rest := strings.TrimPrefix(trimmed, prefix)
		if rest == trimmed {
			prefix = "file://"
			rest = strings.TrimPrefix(trimmed, prefix)
		}
		if rest == "" {
			return v
		}
		rest = strings.TrimLeft(rest, "/")
		if strings.HasPrefix(rest, "~") {
			rel := strings.TrimPrefix(rest, "~")
			abs := filepath.Join(home, rel)
			return prefix + "/" + filepath.ToSlash(strings.TrimLeft(abs, "/"))
		}
	}
	return v
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
	EnsureDefaultAt(context.Background(), afsSvc, root)
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

package workspace

import (
	"os"
	"path/filepath"
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
)

// Predefined kinds.  Callers may still supply arbitrary sub-folder names when
// they need custom separation.
const (
	KindAgent    = "agents"
	KindModel    = "models"
	KindMCP      = "mcp"
	KindWorkflow = "workflows"
)

// Root returns the absolute path to the Agently root directory.
// The lookup order is:
//  1. $AGENTLY_ROOT environment variable, if set and non-empty
//  2. $HOME/.agently
//
// The result is cached for the lifetime of the process.
func Root() string {
	if cachedRoot != "" {
		return cachedRoot
	}

	if env := os.Getenv(envKey); env != "" {
		cachedRoot = abs(env)
		return cachedRoot
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to current working directory on unexpected failure.
		cachedRoot = abs(defaultRootDir)
		return cachedRoot
	}

	cachedRoot = abs(filepath.Join(home, defaultRootDir))
	return cachedRoot
}

// Path returns a sub-path under the root for the given kind (e.g. "agents").
func Path(kind string) string {
	return filepath.Join(Root(), kind)
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

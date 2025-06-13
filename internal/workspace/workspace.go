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
	KindAgent = "agents"
	KindModel = "models"
	KindMCP   = "mcp"
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

// Save writes data to $AGENTLY_ROOT/<kind>/<name>. It creates parent
// directories when missing.
func Save(kind, name string, data []byte, perm os.FileMode) error {
	fullPath := filepath.Join(Path(kind), name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return err
	}
	if perm == 0 {
		perm = 0o660
	}
	return os.WriteFile(fullPath, data, perm)
}

// Delete removes $AGENTLY_ROOT/<kind>/<name>.
func Delete(kind, name string) error {
	return os.Remove(filepath.Join(Path(kind), name))
}

// List lists file basenames stored under $AGENTLY_ROOT/<kind>.
func List(kind string) ([]string, error) {
	dir := Path(kind)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

// Helper wrappers for common kinds ------------------------------------------------

func SaveAgent(name string, data []byte) error { return Save(KindAgent, name, data, 0) }
func DeleteAgent(name string) error            { return Delete(KindAgent, name) }
func ListAgents() ([]string, error)            { return List(KindAgent) }

func SaveModel(name string, data []byte) error { return Save(KindModel, name, data, 0) }
func DeleteModel(name string) error            { return Delete(KindModel, name) }
func ListModels() ([]string, error)            { return List(KindModel) }

func SaveMCP(name string, data []byte) error { return Save(KindMCP, name, data, 0) }
func DeleteMCP(name string) error            { return Delete(KindMCP, name) }
func ListMCP() ([]string, error)             { return List(KindMCP) }

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

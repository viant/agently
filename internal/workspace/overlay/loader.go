package overlay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/viant/agently/internal/workspace"
	"gopkg.in/yaml.v3"
)

var (
	loadOnce sync.Once
	// cached overlays; never mutated after loadOnce executes.
	cached []*Overlay
)

// All returns the list of overlays loaded from the workspace.  The result is
// cached for the lifetime of the process.
func All() []*Overlay {
	loadOnce.Do(func() { cached = load() })
	return cached
}

// load scans "<workspace>/elicitation" and parses every *.yaml|yml|json file
// into an Overlay structure. Invalid files are skipped silently.
func load() []*Overlay {
	root := filepath.Join(workspace.Root(), "elicitation")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var result []*Overlay
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		switch ext {
		case ".yaml", ".yml", ".json":
		default:
			continue
		}

		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}

		var ov Overlay
		if ext == ".json" {
			if err := json.Unmarshal(data, &ov); err != nil {
				continue
			}
		} else {
			// try YAML, fallback to JSON as some .yaml may actually be json
			if err := yaml.Unmarshal(data, &ov); err != nil {
				if json.Unmarshal(data, &ov) != nil {
					continue
				}
			}
		}
		result = append(result, &ov)
	}
	return result
}

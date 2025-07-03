package refiner

// This file introduces a *global* preset facility that allows callers to
// declaratively customise the appearance of every elicitation form generated
// by Agently.  A preset typically originates from a YAML/JSON document that
// lists field-level UI overrides and a preferred ordering.  When present, the
// preset is applied automatically by Refine() – the UI therefore no longer
// needs to call the per-message /elicitation/{id}/refine endpoint.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/viant/agently/internal/workspace"
	"github.com/viant/mcp-protocol/schema"
	yaml "gopkg.in/yaml.v3"
)

// ensurePresetsLoaded scans workspace/elicitation directory once.
func ensurePresetsLoaded() {
	loadOnce.Do(func() {
		root := filepath.Join(workspace.Root(), "elicitation")
		entries, err := os.ReadDir(root)
		if err != nil {
			return // directory may not exist – nothing to load
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".json") {
				_ = tryLoadPreset(filepath.Join(root, e.Name()))
			}
		}
	})
}

// Preset defines optional overrides for elicitation schemas.  Only the
// minimal subset required today is modelled – it can be extended easily when
// new UI hints appear.
type Preset struct {
	// Fields holds per-property overrides.  Each entry must contain a "name"
	// key so that the preset can be matched to the corresponding JSON-Schema
	// property.  Remaining keys are copied verbatim into the property map.
	// A common example is {name: "startDate", widget: "date-picker"}.
	Fields []map[string]any `json:"fields" yaml:"fields"`

	// Match allows to restrict applicability of the preset. When empty the
	// preset always applies. If Match.Fields is non-empty all those field
	// names must exist in the target schema for the preset to be applied.
	Match struct {
		Fields []string `json:"fields,omitempty" yaml:"fields,omitempty"`
	} `json:"match,omitempty" yaml:"match,omitempty"`

	// UI captures root-level overrides such as layout parameters.
	UI map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

// globalPresets lists every preset loaded from workspace/elicitation/*. Access
// is read-only after init(), therefore no extra locking.
var (
	globalPresets []*Preset
	loadOnce      sync.Once
)

// SetGlobalPreset resets the preset registry to a single entry – primarily
// useful in unit tests. Passing nil clears the list.
func SetGlobalPreset(p *Preset) {
	if p == nil {
		globalPresets = nil
	} else {
		globalPresets = []*Preset{p}
	}
}

// init loads an optional preset from the path referenced by the environment
// variable AGENTLY_ELICITATION_PRESET.  Supplying the variable is entirely
// optional – missing/invalid files are ignored so that CLI usage remains
// friction-less.
func init() {
	// Legacy env-var loading remains immediate so tests can inject presets
	// before first Refine() call without touching globalPresets directly.
	if envPath, ok := os.LookupEnv("AGENTLY_ELICITATION_PRESET"); ok && strings.TrimSpace(envPath) != "" {
		_ = tryLoadPreset(envPath)
	}
}

// tryLoadPreset wraps loadPresetFromFile with error logging but no failure.
func tryLoadPreset(path string) error {
	if err := loadPresetFromFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "warn: cannot load elicitation preset %s: %v\n", path, err)
		return err
	}
	return nil
}

// loadPresetFromFile loads a preset from the supplied file path.  The file may
// contain YAML or JSON.
func loadPresetFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Try JSON first – if that fails, fall back to YAML.
	var p Preset
	if err := json.Unmarshal(data, &p); err != nil {
		if err := yaml.Unmarshal(data, &p); err != nil {
			return err
		}
	}
	globalPresets = append(globalPresets, &p)
	return nil
}

// applyPreset mutates rs according to the currently configured preset.  The
// function is intentionally conservative – it only touches properties that
// exist in the schema so that un-related workflows stay unaffected.
func applyPreset(rs *schema.ElicitRequestParamsRequestedSchema) {
	ensurePresetsLoaded()
	if rs == nil || len(globalPresets) == 0 {
		return
	}

	// Shared order counter so later presets can reserve slots.
	seq := 10

	matched := false
	for _, p := range globalPresets {
		if !presetMatchesSchema(p, rs) {
			continue
		}
		if matched {
			// Second match detected – ambiguous. For now we log and skip.
			fmt.Fprintf(os.Stderr, "warn: multiple elicitation presets match the same schema; ignoring %v\n", p)
			continue
		}
		matched = true

		for _, fld := range p.Fields {
			// --- locate target property ----------------------------------
			nameRaw, ok := fld["name"]
			if !ok {
				continue
			}
			name, ok := nameRaw.(string)
			if !ok || name == "" {
				continue
			}

			propAny, ok := rs.Properties[name]
			if !ok {
				continue
			}
			prop, ok := propAny.(map[string]interface{})
			if !ok {
				continue
			}

			// --- merge overrides ----------------------------------------
			for k, v := range fld {
				if k == "name" {
					continue
				}
				prop[k] = v
			}

			if _, has := prop["x-ui-order"]; !has {
				prop["x-ui-order"] = seq
				seq += 10
			}

			rs.Properties[name] = prop
		}
		// After applying first matching preset we exit loop as per contract
		// that at most one preset should match.
		break
	}
}

// presetMatchesSchema decides whether preset p should be applied to rs.
// Rule: when Match.Fields is non-empty all names must be present in rs.Properties.
// Otherwise the preset always applies.
func presetMatchesSchema(p *Preset, rs *schema.ElicitRequestParamsRequestedSchema) bool {
	if p == nil {
		return false
	}
	if len(p.Match.Fields) == 0 {
		return false // requires explicit match list to avoid accidental match
	}

	// Ensure schema properties match exactly the listed fields (order irrelevant).
	if len(p.Match.Fields) != len(rs.Properties) {
		return false
	}
	for _, f := range p.Match.Fields {
		if _, ok := rs.Properties[f]; !ok {
			return false
		}
	}
	return true
}

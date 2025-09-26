package refiner

import (
	"encoding/json"
	"fmt"
	overlaypkg "github.com/viant/agently/internal/overlay"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/mcp-protocol/schema"
	yaml "gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Preset struct {
	Fields []map[string]any `json:"fields" yaml:"fields"`
	Match  struct {
		Fields []string `json:"fields,omitempty" yaml:"fields,omitempty"`
	} `json:"match,omitempty" yaml:"match,omitempty"`
	UI map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

var (
	globalPresets []*Preset
	loadOnce      sync.Once
)

func SetGlobalPreset(p *Preset) {
	if p == nil {
		globalPresets = nil
	} else {
		globalPresets = []*Preset{p}
	}
}
func init() {
	if envPath, ok := os.LookupEnv("AGENTLY_ELICITATION_PRESET"); ok && strings.TrimSpace(envPath) != "" {
		_ = tryLoadPreset(envPath)
	}
}
func ensurePresetsLoaded() {
	loadOnce.Do(func() {
		root := filepath.Join(workspace.Root(), "elicitation")
		entries, err := os.ReadDir(root)
		if err != nil {
			return
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
func tryLoadPreset(path string) error {
	if err := loadPresetFromFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "warn: cannot load elicitation preset %s: %v\n", path, err)
		return err
	}
	return nil
}
func loadPresetFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var p Preset
	if err := json.Unmarshal(data, &p); err != nil {
		if err := yaml.Unmarshal(data, &p); err != nil {
			return err
		}
	}
	globalPresets = append(globalPresets, &p)
	return nil
}
func applyPreset(rs *schema.ElicitRequestParamsRequestedSchema) {
	ensurePresetsLoaded()
	if rs == nil || len(globalPresets) == 0 {
		return
	}
	seq := 10
	matched := false
	for _, p := range globalPresets {
		if !presetMatchesSchema(p, rs) {
			continue
		}
		if matched {
			fmt.Fprintf(os.Stderr, "warn: multiple elicitation presets match the same schema; ignoring %v\n", p)
			continue
		}
		matched = true
		for _, fld := range p.Fields {
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
		break
	}
}
func presetMatchesSchema(p *Preset, rs *schema.ElicitRequestParamsRequestedSchema) bool {
	if p == nil || len(p.Match.Fields) == 0 {
		return false
	}
	tmp := make(map[string]any, len(rs.Properties))
	for k := range rs.Properties {
		tmp[k] = struct{}{}
	}
	return overlaypkg.FieldsMatch(tmp, p.Match.Fields, true)
}

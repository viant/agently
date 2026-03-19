package v1

import (
	"embed"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/viant/agently-core/workspace"
	"gopkg.in/yaml.v3"
)

//go:embed defaults/config.yaml defaults/tools defaults/models defaults/embedders defaults/agents/chatter defaults/agents/coder
var defaultsFS embed.FS

var defaultSeedAgents = []string{"chatter", "coder"}

func setBootstrapHook() {
	workspace.SetBootstrapHook(func(store *workspace.BootstrapStore) error {
		if err := ensureWorkspaceDirs(store.Root()); err != nil {
			return err
		}
		if err := seedFileIfMissing(store.Root(), "config.yaml", "defaults/config.yaml"); err != nil {
			return err
		}
		for _, seed := range []struct {
			src  string
			dest string
		}{
			{src: "defaults/tools", dest: "tools"},
			{src: "defaults/models", dest: "models"},
			{src: "defaults/embedders", dest: "embedders"},
		} {
			if err := seedTreeIfMissing(store.Root(), seed.src, seed.dest); err != nil {
				return err
			}
		}
		for _, agent := range defaultSeedAgents {
			src := filepath.ToSlash(filepath.Join("defaults", "agents", agent))
			dest := filepath.ToSlash(filepath.Join("agents", agent))
			if err := seedTreeIfMissing(store.Root(), src, dest); err != nil {
				return err
			}
		}
		if err := ensureInternalMCPConfig(filepath.Join(store.Root(), "config.yaml")); err != nil {
			return err
		}
		if err := ensureDefaultWorkspaceConfig(filepath.Join(store.Root(), "config.yaml")); err != nil {
			return err
		}
		return nil
	})
}

// skipBootstrapDirs are workspace kinds that should not be eagerly created
// at startup. They are created on-demand when the subsystem actually needs them.
var skipBootstrapDirs = map[string]bool{
	workspace.KindMCP: true,
}

func ensureWorkspaceDirs(root string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	for _, kind := range workspace.AllKinds() {
		if skipBootstrapDirs[kind] {
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, kind), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func seedFileIfMissing(root, targetPath, sourcePath string) error {
	absTarget := filepath.Join(root, targetPath)
	if _, err := os.Stat(absTarget); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := iofs.ReadFile(defaultsFS, sourcePath)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absTarget, data, 0o644)
}

func seedTreeIfMissing(root, sourcePrefix, targetPrefix string) error {
	sourcePrefix = strings.Trim(strings.TrimSpace(filepath.ToSlash(sourcePrefix)), "/")
	targetPrefix = strings.Trim(strings.TrimSpace(filepath.ToSlash(targetPrefix)), "/")
	if sourcePrefix == "" || targetPrefix == "" {
		return nil
	}
	return iofs.WalkDir(defaultsFS, "defaults", func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != sourcePrefix && !strings.HasPrefix(path, sourcePrefix+"/") {
			return nil
		}
		rel := strings.TrimPrefix(path, sourcePrefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		target := filepath.Join(root, filepath.FromSlash(targetPrefix), filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if _, statErr := os.Stat(target); statErr == nil {
			return nil
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		data, readErr := iofs.ReadFile(defaultsFS, path)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(target), 0o755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func ensureInternalMCPConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	root := map[string]interface{}{}
	if err = yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	targetServices := []string{"system/exec", "system/os", "system/patch", "orchestration/plan", "llm/agents", "resources", "internal/message"}
	changed := false

	// Migrate legacy flat key if present.
	existing := valuesToStrings(root["internalMCPServices"])
	if len(existing) > 0 {
		merged := mergeServiceNames(existing, targetServices)
		if !equalStringSlices(existing, merged) {
			root["internalMCPServices"] = merged
			changed = true
		}
	}

	// Only seed the internalMCP.services section if it does not already exist
	// in config.yaml. If the user has defined (or intentionally removed) the
	// section, respect their choice and do not force-merge defaults.
	internal, ok := root["internalMCP"].(map[string]interface{})
	if !ok || internal == nil {
		internal = map[string]interface{}{"services": targetServices}
		root["internalMCP"] = internal
		changed = true
	} else if internal["services"] == nil {
		// Section exists but services key is missing — seed defaults.
		internal["services"] = targetServices
		changed = true
	}
	// If internalMCP.services already has values, leave them as-is.

	if !changed {
		return nil
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func valuesToStrings(value interface{}) []string {
	switch actual := value.(type) {
	case []string:
		return append([]string{}, actual...)
	case []interface{}:
		result := make([]string, 0, len(actual))
		for _, item := range actual {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result
	case string:
		if strings.TrimSpace(actual) == "" {
			return nil
		}
		parts := strings.Split(actual, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := strings.TrimSpace(part); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func mergeServiceNames(existing, required []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(existing)+len(required))
	for _, item := range existing {
		name := normalizeServiceName(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	for _, item := range required {
		name := normalizeServiceName(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if normalizeServiceName(left[i]) != normalizeServiceName(right[i]) {
			return false
		}
	}
	return true
}

func ensureDefaultWorkspaceConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	root := map[string]interface{}{}
	if err = yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	changed := false

	defaults, ok := root["default"].(map[string]interface{})
	if !ok || defaults == nil {
		defaults = map[string]interface{}{}
		root["default"] = defaults
		changed = true
	}
	if strings.TrimSpace(asString(defaults["agent"])) == "" {
		defaults["agent"] = "chatter"
		changed = true
	}
	if strings.TrimSpace(asString(defaults["model"])) == "" {
		defaults["model"] = "openai_gpt-5.2"
		changed = true
	}
	if strings.TrimSpace(asString(defaults["embedder"])) == "" {
		defaults["embedder"] = "openai_text"
		changed = true
	}

	for _, section := range []string{"models", "embedders", "agents"} {
		ref, ok := root[section].(map[string]interface{})
		if !ok || ref == nil {
			root[section] = map[string]interface{}{"url": section}
			changed = true
			continue
		}
		if strings.TrimSpace(asString(ref["url"])) == "" {
			ref["url"] = section
			changed = true
		}
	}

	if !changed {
		return nil
	}
	updated, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644)
}

func asString(value interface{}) string {
	switch actual := value.(type) {
	case string:
		return actual
	default:
		return ""
	}
}

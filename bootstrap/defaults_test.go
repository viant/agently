package bootstrap

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultAgentsExposeExactMessageAdd(t *testing.T) {
	for _, path := range []string{
		"defaults/agents/chatter/chatter.yaml",
		"defaults/agents/coder/coder.yaml",
	} {
		data, err := DefaultsFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		var cfg struct {
			Tool struct {
				Bundles []string `yaml:"bundles"`
				Items   []struct {
					Name string `yaml:"name"`
				} `yaml:"items"`
			} `yaml:"tool"`
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if !hasToolItem(cfg.Tool.Items, "message:add") {
			t.Fatalf("%s: expected exact message:add tool item", path)
		}
		if hasBundle(cfg.Tool.Bundles, "message") {
			t.Fatalf("%s: expose message:add explicitly instead of the broad message bundle", path)
		}
	}
}

func hasToolItem(items []struct {
	Name string `yaml:"name"`
}, name string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), name) {
			return true
		}
	}
	return false
}

func hasBundle(bundles []string, bundle string) bool {
	for _, item := range bundles {
		if strings.EqualFold(strings.TrimSpace(item), bundle) {
			return true
		}
	}
	return false
}

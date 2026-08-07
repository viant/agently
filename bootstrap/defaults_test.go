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

func TestDefaultWorkspaceConfigEnablesSystemGoal(t *testing.T) {
	data, err := DefaultsFS.ReadFile("defaults/config.yaml")
	if err != nil {
		t.Fatalf("read defaults/config.yaml: %v", err)
	}

	var cfg struct {
		InternalMCP struct {
			Services []string `yaml:"services"`
		} `yaml:"internalMCP"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse defaults/config.yaml: %v", err)
	}
	if !hasBundle(cfg.InternalMCP.Services, "system/goal") {
		t.Fatalf("defaults/config.yaml: expected system/goal internal MCP service")
	}
}

func TestDefaultWorkspaceIncludesGenericBedrockQwen(t *testing.T) {
	data, err := DefaultsFS.ReadFile("defaults/models/bedrock_qwen3-coder-next.yaml")
	if err != nil {
		t.Fatalf("read Bedrock Qwen default: %v", err)
	}
	var cfg struct {
		Options struct {
			Provider       string `yaml:"provider"`
			Model          string `yaml:"model"`
			Region         string `yaml:"region"`
			CredentialsURL string `yaml:"credentialsURL"`
			MaxTokens      int    `yaml:"maxTokens"`
		} `yaml:"options"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse Bedrock Qwen default: %v", err)
	}
	if cfg.Options.Provider != "bedrock" || cfg.Options.Model != "qwen.qwen3-coder-next" || cfg.Options.Region != "us-east-1" {
		t.Fatalf("unexpected Bedrock Qwen options: provider=%q model=%q region=%q", cfg.Options.Provider, cfg.Options.Model, cfg.Options.Region)
	}
	if cfg.Options.CredentialsURL != "aws-bedrock-qwen|blowfish://default" {
		t.Fatalf("expected generic scy resource reference, got %q", cfg.Options.CredentialsURL)
	}
	if cfg.Options.MaxTokens != 16384 {
		t.Fatalf("expected Qwen3 Coder Next 16K output limit, got %d", cfg.Options.MaxTokens)
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

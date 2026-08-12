package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSelectGeneratedWorkspaceModelUsesAvailableBundledProvider(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "OpenAI remains preferred", env: map[string]string{"OPENAI_API_KEY": "configured", "XAI_API_KEY": "configured"}, want: "openai_gpt-5.4"},
		{name: "xAI fallback", env: map[string]string{"XAI_API_KEY": "configured"}, want: "xai_grok-4-latest"},
		{name: "Gemini fallback", env: map[string]string{"GEMINI_API_KEY": "configured"}, want: "vertexai_gemini_3_0_pro"},
		{name: "documented default without credentials", env: map[string]string{}, want: "openai_gpt-5.4"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := selectGeneratedWorkspaceModel(func(key string) string { return test.env[key] })
			if got != test.want {
				t.Fatalf("model = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConfigureGeneratedWorkspaceModelsRewritesConfigAndAgents(t *testing.T) {
	root := t.TempDir()
	for _, seed := range []struct{ source, target string }{
		{"defaults/config.yaml", "config.yaml"},
		{"defaults/agents/chatter/chatter.yaml", "agents/chatter/chatter.yaml"},
		{"defaults/agents/coder/coder.yaml", "agents/coder/coder.yaml"},
	} {
		data, err := DefaultsFS.ReadFile(seed.source)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, seed.target)
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := configureGeneratedWorkspaceModels(root, func(key string) string {
		if key == "XAI_API_KEY" {
			return "configured"
		}
		return ""
	}); err != nil {
		t.Fatal(err)
	}

	configData, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Default struct {
			Model              string `yaml:"model"`
			SummaryModel       string `yaml:"summaryModel"`
			AgentAutoSelection struct {
				Model string `yaml:"model"`
			} `yaml:"agentAutoSelection"`
			ToolAutoSelection struct {
				Model string `yaml:"model"`
			} `yaml:"toolAutoSelection"`
		} `yaml:"default"`
	}
	if err = yaml.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]string{
		"default":         config.Default.Model,
		"summary":         config.Default.SummaryModel,
		"agent selection": config.Default.AgentAutoSelection.Model,
		"tool selection":  config.Default.ToolAutoSelection.Model,
	} {
		if got != "xai_grok-4-latest" {
			t.Fatalf("%s model = %q", name, got)
		}
	}
	for _, agent := range []string{"chatter", "coder"} {
		data, readErr := os.ReadFile(filepath.Join(root, "agents", agent, agent+".yaml"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var cfg struct {
			ModelRef string `yaml:"modelRef"`
			Intake   struct {
				Model string `yaml:"model"`
			} `yaml:"intake"`
		}
		if err = yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.ModelRef != "xai_grok-4-latest" || cfg.Intake.Model != "xai_grok-4-latest" {
			t.Fatalf("%s models = modelRef %q, intake %q", agent, cfg.ModelRef, cfg.Intake.Model)
		}
	}
}

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

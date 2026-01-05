package workspace

import (
	"bytes"
	"context"
	"embed"
	"log"
	"path/filepath"

	"github.com/viant/afs"
	_ "github.com/viant/afs/embed"
	"github.com/viant/afs/file"
	"github.com/viant/afs/url"
)

//go:embed default/*
var defaultsFS embed.FS

// EnsureDefault
func EnsureDefault(fs afs.Service) {

	ctx := context.Background()
	entries := []struct {
		path string // relative to workspace root
		src  string // path inside embed FS default/
	}{
		{"config.yaml", "default/config.yaml"},

		{filepath.Join(KindToolHints, "webdriver.md"), "default/tools/hints/webdriver.md"},

		{filepath.Join(KindToolBundle, "resources.yaml"), "default/tools/bundles/resources.yaml"},
		{filepath.Join(KindToolBundle, "system_exec.yaml"), "default/tools/bundles/system_exec.yaml"},
		{filepath.Join(KindToolBundle, "system_patch.yaml"), "default/tools/bundles/system_patch.yaml"},
		{filepath.Join(KindToolBundle, "system_os.yaml"), "default/tools/bundles/system_os.yaml"},
		{filepath.Join(KindToolBundle, "system_image.yaml"), "default/tools/bundles/system_image.yaml"},
		{filepath.Join(KindToolBundle, "agents.yaml"), "default/tools/bundles/agents.yaml"},
		{filepath.Join(KindToolBundle, "agent_exec.yaml"), "default/tools/bundles/agent_exec.yaml"},
		{filepath.Join(KindToolBundle, "orchestration.yaml"), "default/tools/bundles/orchestration.yaml"},
		{filepath.Join(KindToolBundle, "message.yaml"), "default/tools/bundles/message.yaml"},
		{filepath.Join(KindToolBundle, "webdriver.yaml"), "default/tools/bundles/webdriver.yaml"},
		{filepath.Join(KindToolBundle, "sqlkit.yaml"), "default/tools/bundles/sqlkit.yaml"},
		{filepath.Join(KindToolBundle, "github.yaml"), "default/tools/bundles/github.yaml"},
		{filepath.Join(KindToolBundle, "platform.yaml"), "default/tools/bundles/platform.yaml"},
		{filepath.Join(KindToolBundle, "outlook.yaml"), "default/tools/bundles/outlook.yaml"},

		{filepath.Join(KindFeeds, "changes.yaml"), "default/feeds/changes.yaml"},
		{filepath.Join(KindFeeds, "plan.yaml"), "default/feeds/plan.yaml"},
		{filepath.Join(KindFeeds, "terminal.yaml"), "default/feeds/terminal.yaml"},
		{filepath.Join(KindFeeds, "explorer.yaml"), "default/feeds/explorer.yaml"},

		{filepath.Join(KindEmbedder, "openai_text.yaml"), "default/model/openai/embedder_text.yaml"},
		{filepath.Join(KindModel, "openai_o4-mini.yaml"), "default/model/openai/o4-mini.yaml"},
		{filepath.Join(KindModel, "openai_o3.yaml"), "default/model/openai/o3.yaml"},
		{filepath.Join(KindModel, "openai_gpt-5.yaml"), "default/model/openai/gpt-5.yaml"},
		{filepath.Join(KindModel, "openai_gpt-5_1.yaml"), "default/model/openai/gpt-5_1.yaml"},
		{filepath.Join(KindModel, "openai_gpt-5_2.yaml"), "default/model/openai/gpt-5_2.yaml"},
		{filepath.Join(KindModel, "xai_grok-4-latest.yaml"), "default/model/xai/grok_4_latest.yaml"},
		{filepath.Join(KindModel, "xai_grok-code-fast-1.yaml"), "default/model/xai/grok_code_fast_1.yaml"},

		{filepath.Join(KindModel, "bedrock_claude_3-7.yaml"), "default/model/bedrock/claude_3-7.yaml"},
		{filepath.Join(KindModel, "vertexai_claude_opus_4.yaml"), "default/model/vertexai/claude_opus_4.yaml"},
		{filepath.Join(KindModel, "vertexai_gemini_flash2_5.yaml"), "default/model/vertexai/gemini_flash2_5.yaml"},
		{filepath.Join(KindModel, "vertexai_gemini_2_5_pro.yaml"), "default/model/vertexai/gemini_2_5_pro.yaml"},
		{filepath.Join(KindModel, "vertexai_gemini_3_0_pro.yaml"), "default/model/vertexai/gemini_3_0_pro.yaml"},

		{filepath.Join(KindAgent, "chatter/knowledge/doc.txt"), "default/agents/chatter/knowledge/doc.txt"},
		{filepath.Join(KindAgent, "chatter/prompt/system.tmpl"), "default/agents/chatter/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "chatter/prompt/user.tmpl"), "default/agents/chatter/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "chatter/chatter.yaml"), "default/agents/chatter/chatter.yaml"},

		{filepath.Join(KindAgent, "coder/prompt/system.tmpl"), "default/agents/coder/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "coder/prompt/user.tmpl"), "default/agents/coder/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "coder/system_knowledge/golang_rules.md"), "default/agents/coder/system_knowledge/golang_rules.md"},
		{filepath.Join(KindAgent, "coder/coder.yaml"), "default/agents/coder/coder.yaml"},

		{filepath.Join(KindAgent, "dev_verifier/prompt/system.tmpl"), "default/agents/dev_verifier/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "dev_verifier/prompt/user.tmpl"), "default/agents/dev_verifier/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "dev_verifier/dev_verifier.yaml"), "default/agents/dev_verifier/dev_verifier.yaml"},

		{filepath.Join(KindAgent, "dev_composer/prompt/system.tmpl"), "default/agents/dev_composer/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "dev_composer/prompt/user.tmpl"), "default/agents/dev_composer/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "dev_composer/dev_composer.yaml"), "default/agents/dev_composer/dev_composer.yaml"},

		{filepath.Join(KindAgent, "dev_orchestrator/prompt/system.tmpl"), "default/agents/dev_orchestrator/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "dev_orchestrator/prompt/user.tmpl"), "default/agents/dev_orchestrator/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "dev_orchestrator/dev_orchestrator.yaml"), "default/agents/dev_orchestrator/dev_orchestrator.yaml"},

		{filepath.Join(KindAgent, "critic/prompt/system.tmpl"), "default/agents/critic/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "critic/prompt/user.tmpl"), "default/agents/critic/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "critic/critic.yaml"), "default/agents/critic/critic.yaml"},
	}

	baseURL := url.Normalize(Root(), file.Scheme)
	for _, e := range entries {
		absPath := url.Join(baseURL, e.path)
		// Skip if already present
		if ok, _ := fs.Exists(ctx, absPath); ok {
			continue
		}

		data, err := fs.DownloadWithURL(ctx, url.Join("embed://localhost/", e.src), &defaultsFS)
		if err != nil {
			log.Printf("[workspace] failed to download %v: %v", e.src, err)
			continue
		}
		if err := fs.Upload(ctx, absPath, file.DefaultFileOsMode, bytes.NewReader(data)); err != nil {
			log.Printf("[workspace] failed to upload %v to %v: %v", e.src, absPath, err)
		}
	}
}

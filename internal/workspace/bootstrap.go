package workspace

import (
	"bytes"
	"context"
	"embed"
	"fmt"
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

		{filepath.Join(KindEmbedder, "openai_text.yaml"), "default/model/openai/embedder_text.yaml"},
		{filepath.Join(KindModel, "openai_o4-mini.yaml"), "default/model/openai/o4-mini.yaml"},
		{filepath.Join(KindModel, "openai_o3.yaml"), "default/model/openai/o3.yaml"},
		{filepath.Join(KindModel, "openai_gpt-5.yaml"), "default/model/openai/gpt-5.yaml"},

		{filepath.Join(KindModel, "bedrock_claude_3_7.yaml"), "default/model/bedrock/claude_3_7.yaml"},
		{filepath.Join(KindModel, "vertexai_claude_opus_4.yaml"), "default/model/vertexai/claude_opus_4.yaml"},
		{filepath.Join(KindModel, "vertexai_gemini_flash2_5.yaml"), "default/model/vertexai/gemini_flash2_5.yaml"},

		{filepath.Join(KindAgent, "chat/knowledge/doc.txt"), "default/agents/chat/knowledge/doc.txt"},
		{filepath.Join(KindAgent, "chat/knowledge/README_DELETE_THIS.md"), "default/agents/chat/knowledge/README_DELETE_THIS.md"},
		{filepath.Join(KindAgent, "chat/prompt/system.tmpl"), "default/agents/chat/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "chat/prompt/user.tmpl"), "default/agents/chat/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "chat/system_knowledge/golang_rules.md"), "default/agents/chat/system_knowledge/golang_rules.md"},
		{filepath.Join(KindAgent, "chat/system_knowledge/README_DELETE_THIS.md"), "default/agents/chat/system_knowledge/README_DELETE_THIS.md"},
		{filepath.Join(KindAgent, "chat/chat.yaml"), "default/agents/chat/chat.yaml"},

		{filepath.Join(KindAgent, "coder/knowledge/README_DELETE_THIS.md"), "default/agents/coder/knowledge/README_DELETE_THIS.md"},
		{filepath.Join(KindAgent, "coder/prompt/system.tmpl"), "default/agents/coder/prompt/system.tmpl"},
		{filepath.Join(KindAgent, "coder/prompt/user.tmpl"), "default/agents/coder/prompt/user.tmpl"},
		{filepath.Join(KindAgent, "coder/system_knowledge/golang_rules.md"), "default/agents/coder/system_knowledge/golang_rules.md"},
		{filepath.Join(KindAgent, "coder/system_knowledge/README_DELETE_THIS.md"), "default/agents/coder/system_knowledge/README_DELETE_THIS.md"},
		{filepath.Join(KindAgent, "coder/coder.yaml"), "default/agents/coder/coder.yaml"},
	}

	baseURL := url.Normalize(Root(), file.Scheme)
	absPath := url.Join(baseURL, entries[0].path)
	if ok, _ := fs.Exists(ctx, absPath); ok {
		//config already exists skipping default workspace creation
		return
	}
	for _, e := range entries {
		absPath := url.Join(baseURL, e.path)
		// Skip if already present
		if ok, _ := fs.Exists(ctx, absPath); ok {
			continue
		}

		data, err := fs.DownloadWithURL(ctx, url.Join("embed://localhost/", e.src), &defaultsFS)
		if err != nil {
			fmt.Printf("failed to download %v: %v\n", e.src, err)
			continue
		}
		_ = fs.Upload(ctx, absPath, file.DefaultFileOsMode, bytes.NewReader(data))
	}
}

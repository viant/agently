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

// ensureDefault populates the workspace root with minimal default resources
// (config, agent, model, workflow) when they are missing so that first-time
// users have a ready-to-use environment without manual setup.
// EnsureDefault initialises a fresh workspace with baseline resources when
// they are missing. It is safe to call multiple times – it only writes files
// that do not yet exist.
func EnsureDefault(fs afs.Service) {

	ctx := context.Background()

	entries := []struct {
		path string // relative to workspace root
		src  string // path inside embed FS default/
	}{
		{"config.yaml", "default/config.yaml"},
		{filepath.Join(KindAgent, "chat/chat.yaml"), "default/agent_chat.yaml"},

		{filepath.Join(KindModel, "o4-mini.yaml"), "default/model_o4-mini.yaml"},
		{filepath.Join(KindModel, "o3.yaml"), "default/model_o3.yaml"},
		{filepath.Join(KindEmbedder, "text.yaml"), "default/embedder_text.yaml"},

		{filepath.Join(KindAgent, "chat/workflows/orchestration.yaml"), "default/agent_chat_workflow_orchestration.yaml"},
		{filepath.Join(KindAgent, "chat/workflows/prompt", "chat.vm"), "default/agent_chat_prompt.vm"},
		{filepath.Join(KindAgent, "chat/knowledge/doc.txt"), "default/agent_chat_doc.txt"},
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

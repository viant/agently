package workspace

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
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
	// Skip auto-bootstrapping when AGENTLY_ROOT is explicitly set. This gives
	// callers full control over workspace contents (for example, unit tests that
	// start with an empty repository).
	if os.Getenv(envKey) != "" {
		return
	}
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

		{filepath.Join(KindAgent, "chat/workflows/orchestration.yaml"), "default/workflow_orchestration.yaml"},
		{filepath.Join(KindAgent, "chat/workflows/prompt", "plan.vm"), "default/plan.vm"},
		{filepath.Join(KindAgent, "chat/knowledge/doc.txt"), "default/doc.txt"},
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
			fmt.Printf("failed to download %v: %v\n", e.src, err)
			continue
		}
		_ = fs.Upload(ctx, absPath, file.DefaultFileOsMode, bytes.NewReader(data))
	}
}

package workspace

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Respect explicit workspace override via $AGENTLY_ROOT: when the variable is
	// set callers expect a clean workspace without auto-populated defaults (for
	// example unit tests using t.TempDir()).
	if env := os.Getenv(envKey); strings.TrimSpace(env) != "" {
		return
	}

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
		{filepath.Join(KindAgent, "chat/workflows/prompt", "chat.vm"), "default/agent_chat_prompt.txt"},
		{filepath.Join(KindAgent, "chat/knowledge/doc.txt"), "default/agent_chat_doc.txt"},
		{filepath.Join(KindAgent, "coder/workflows/orchestration.yaml"), "default/agent_coder_workflow_orchestration.yaml"},
		{filepath.Join(KindAgent, "coder/workflows/prompt", "chat.vm"), "default/coder_chat_prompt.txt"},
		{filepath.Join(KindAgent, "coder/knowledge/golang.md"), "default/coder_knowledge_golang.md"},
		{filepath.Join(KindAgent, "coder/coder.yaml"), "default/agent_coder.yaml"},
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

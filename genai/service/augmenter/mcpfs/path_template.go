package mcpfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/viant/afs/url"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/agently/internal/workspace"
)

const (
	envSnapshotPath     = "AGENTLY_SNAPSHOT_PATH"
	snapshotPathDefault = "${runtimeRoot}/snapshots"
)

func (s *Service) snapshotCacheRoot(ctx context.Context) string {
	template := strings.TrimSpace(s.snapshotRoot)
	if envVal := strings.TrimSpace(os.Getenv(envSnapshotPath)); envVal != "" {
		template = envVal
	}
	if template == "" {
		template = snapshotPathDefault
	}
	user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	if user == "" {
		user = "default"
	}
	out := strings.ReplaceAll(template, "${user}", user)
	out = strings.ReplaceAll(out, "${workspaceRoot}", workspace.Root())
	out = strings.ReplaceAll(out, "${runtimeRoot}", workspace.RuntimeRoot())
	out = strings.TrimSpace(out)
	if strings.HasPrefix(strings.ToLower(out), "file://") {
		out = fileURLToPath(out)
	}
	if filepath.IsAbs(out) || isWindowsAbsPath(out) {
		return filepath.Clean(out)
	}
	return filepath.Clean(filepath.Join(workspace.Root(), out))
}

func fileURLToPath(u string) string {
	u = strings.TrimSpace(u)
	if !strings.HasPrefix(u, "file://") {
		return u
	}
	rest := strings.TrimPrefix(u, "file://")
	rest = strings.TrimPrefix(rest, "localhost")
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return url.Path(rest)
}

func isWindowsAbsPath(v string) bool {
	if len(v) < 2 {
		return false
	}
	if v[1] != ':' {
		return false
	}
	if v[0] >= 'a' && v[0] <= 'z' || v[0] >= 'A' && v[0] <= 'Z' {
		return true
	}
	return false
}

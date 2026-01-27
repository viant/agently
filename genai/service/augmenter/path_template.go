package augmenter

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
	envIndexPath     = "AGENTLY_INDEX_PATH"
	indexPathDefault = "${runtimeRoot}/index/${user}"
)

type indexPathKey struct{}

// WithIndexPathTemplateContext stores an index path template in context.
func WithIndexPathTemplateContext(ctx context.Context, template string) context.Context {
	if ctx == nil || strings.TrimSpace(template) == "" {
		return ctx
	}
	return context.WithValue(ctx, indexPathKey{}, template)
}

func indexPathTemplateFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(indexPathKey{}).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func resolveIndexBaseURL(ctx context.Context, template string) string {
	resolved := resolveTemplate(ctx, envIndexPath, template, indexPathDefault)
	return normalizePath(resolved)
}

// ResolveIndexPath resolves an index path template using env overrides and defaults.
func ResolveIndexPath(ctx context.Context, template string) string {
	return resolveTemplate(ctx, envIndexPath, template, indexPathDefault)
}

func resolveTemplate(ctx context.Context, envKey, template, fallback string) string {
	if envVal := strings.TrimSpace(getenv(envKey)); envVal != "" {
		template = envVal
	}
	if strings.TrimSpace(template) == "" {
		template = fallback
	}
	user := strings.TrimSpace(authctx.EffectiveUserID(ctx))
	if user == "" {
		user = "default"
	}
	out := strings.ReplaceAll(template, "${user}", user)
	out = strings.ReplaceAll(out, "${workspaceRoot}", workspace.Root())
	out = strings.ReplaceAll(out, "${runtimeRoot}", workspace.RuntimeRoot())
	out = workspace.ResolvePathTemplate(out)
	out = strings.ReplaceAll(out, "${user}", user)
	return strings.TrimSpace(out)
}

func normalizePath(p string) string {
	if strings.TrimSpace(p) == "" {
		return p
	}
	if strings.HasPrefix(strings.ToLower(p), "file://") {
		p = fileURLToPath(p)
	}
	if filepath.IsAbs(p) || isWindowsAbsPath(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(workspace.Root(), p))
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

func getenv(key string) string {
	if key == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(key))
}

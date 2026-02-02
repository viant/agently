package augmenter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/afs/url"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/embedius/indexer/fs"
	"github.com/viant/embedius/indexer/fs/splitter"
	"github.com/viant/embedius/matching"
	"github.com/viant/embedius/vectordb/sqlitevec"
)

func TestUpstreamSyncConfig_LocalRoot(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("AGENTLY_WORKSPACE", tempDir)
	t.Setenv("AGENTLY_WORKSPACE_NO_DEFAULTS", "1")

	matcher := matching.New()
	splitterFactory := splitter.NewFactory(4096)
	fsIndexer := fs.New(tempDir, "test", matcher, splitterFactory)
	aug := &DocsAugmenter{fsIndexer: fsIndexer, store: &sqlitevec.Store{}}

	disabled := false
	svc := New(nil, WithLocalUpstreams(
		[]LocalRoot{{ID: "docs", URI: "workspace://localhost/docs", UpstreamRef: "local"}},
		[]LocalUpstream{{Name: "local", Enabled: &disabled}},
	))

	base := url.Normalize(workspace.Root(), "file")
	location := url.Join(base, "docs")

	cfg := svc.upstreamSyncConfig(context.Background(), location, aug)
	require.NotNil(t, cfg)
	require.False(t, cfg.Enabled)
}

func TestUpstreamSyncConfig_ContextRoots(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("AGENTLY_WORKSPACE", tempDir)
	t.Setenv("AGENTLY_WORKSPACE_NO_DEFAULTS", "1")

	matcher := matching.New()
	splitterFactory := splitter.NewFactory(4096)
	fsIndexer := fs.New(tempDir, "test", matcher, splitterFactory)
	aug := &DocsAugmenter{fsIndexer: fsIndexer, store: &sqlitevec.Store{}}

	disabled := false
	svc := New(nil, WithLocalUpstreams(
		nil,
		[]LocalUpstream{{Name: "local", Enabled: &disabled}},
	))

	base := url.Normalize(workspace.Root(), "file")
	location := url.Join(base, "docs")
	ctx := WithLocalRoots(context.Background(), []LocalRoot{{URI: "workspace://localhost/docs", UpstreamRef: "local"}})

	cfg := svc.upstreamSyncConfig(ctx, location, aug)
	require.NotNil(t, cfg)
	require.False(t, cfg.Enabled)
}

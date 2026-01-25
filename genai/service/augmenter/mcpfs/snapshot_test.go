package mcpfs

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
)

func TestSnapshotManifestDisabled(t *testing.T) {
	workspaceDir := t.TempDir()
	t.Setenv("AGENTLY_WORKSPACE", workspaceDir)
	t.Setenv("AGENTLY_WORKSPACE_NO_DEFAULTS", "1")

	snapURI := "mcp:github://github.vianttech.com/adelphic/mediator/_snapshot.zip"
	rootURI := "mcp:github://github.vianttech.com/adelphic/mediator"
	zipPath := snapshotCachePath(snapURI)
	require.NotEmpty(t, zipPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(t, writeZip(zipPath, map[string]string{
		"adelphic-mediator/file.txt": "hello",
	}))

	svc := New(&mcpmgr.Manager{},
		WithSnapshotResolver(func(string) (string, string, bool) {
			return snapURI, rootURI, true
		}),
		WithSnapshotManifestResolver(func(string) bool { return false }),
	)
	objects, err := svc.listSnapshot(context.Background(), rootURI, snapURI, rootURI)
	require.NoError(t, err)
	require.NotEmpty(t, objects)

	manifestFile := manifestPath(&snapshotCache{path: zipPath})
	if _, err := os.Stat(manifestFile); err == nil {
		t.Fatalf("expected no manifest file, got %s", manifestFile)
	}
	for _, obj := range objects {
		if withMD5, ok := obj.(interface{ MD5() string }); ok {
			require.Equal(t, "", withMD5.MD5())
		}
	}
}

func TestSnapshotManifestEnabled(t *testing.T) {
	workspaceDir := t.TempDir()
	t.Setenv("AGENTLY_WORKSPACE", workspaceDir)
	t.Setenv("AGENTLY_WORKSPACE_NO_DEFAULTS", "1")

	snapURI := "mcp:github://github.vianttech.com/adelphic/mediator/_snapshot.zip"
	rootURI := "mcp:github://github.vianttech.com/adelphic/mediator"
	zipPath := snapshotCachePath(snapURI)
	require.NotEmpty(t, zipPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(t, writeZip(zipPath, map[string]string{
		"adelphic-mediator/file.txt": "hello",
	}))

	svc := New(&mcpmgr.Manager{},
		WithSnapshotResolver(func(string) (string, string, bool) {
			return snapURI, rootURI, true
		}),
		WithSnapshotManifestResolver(func(string) bool { return true }),
	)
	objects, err := svc.listSnapshot(context.Background(), rootURI, snapURI, rootURI)
	require.NoError(t, err)
	require.NotEmpty(t, objects)

	manifestFile := manifestPath(&snapshotCache{path: zipPath})
	if _, err := os.Stat(manifestFile); err != nil {
		t.Fatalf("expected manifest file %s to exist: %v", manifestFile, err)
	}
	foundMD5 := false
	for _, obj := range objects {
		if withMD5, ok := obj.(interface{ MD5() string }); ok && withMD5.MD5() != "" {
			foundMD5 = true
			break
		}
	}
	if !foundMD5 {
		t.Fatalf("expected at least one object with md5")
	}
}

func writeZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, data := range files {
		h := &zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: time.Now().UTC(),
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := w.Write([]byte(data)); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

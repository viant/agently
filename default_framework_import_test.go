package agently

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/viant/afs"
	"github.com/viant/agently/metadata"
	"github.com/viant/forge/backend/handlers"
	"github.com/viant/forge/backend/service/meta"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameworkConversationOriginalLoaderImportRepairPreservesTargets(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, fs.WalkDir(metadata.FS, "window", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(root, file), 0700)
		}
		data, err := fs.ReadFile(metadata.FS, file)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, file), data, 0600)
	}))
	base := "file://" + filepath.Join(root, "window")
	loader := meta.New(afs.New(), base)
	targets := []*meta.TargetContext{{Platform: "web"}, {Platform: "ios", FormFactor: "phone"}, {Platform: "android", FormFactor: "phone"}}
	fixedTargets := make([][]byte, len(targets))
	for i, target := range targets {
		window, err := handlers.LoadWindow(context.Background(), loader, base, "workflow", "conversation", target)
		require.NoError(t, err)
		fixedTargets[i], err = json.Marshal(window)
		require.NoError(t, err)
	}
	file := filepath.Join(root, "window/workflow/conversation/main.yaml")
	fixed, err := os.ReadFile(file)
	require.NoError(t, err)
	original := strings.ReplaceAll(string(fixed), "../../chat/conversations/", "../../../chat/conversations/")
	require.NotEqual(t, string(fixed), original)
	require.NoError(t, os.WriteFile(file, []byte(original), 0600))
	_, err = handlers.LoadWindow(context.Background(), loader, base, "workflow", "conversation", nil)
	require.Error(t, err, "original untargeted relative import must reproduce failure")
	for i, target := range targets {
		window, err := handlers.LoadWindow(context.Background(), loader, base, "workflow", "conversation", target)
		require.NoError(t, err)
		raw, err := json.Marshal(window)
		require.NoError(t, err)
		require.JSONEq(t, string(fixedTargets[i]), string(raw), "target-specific original loader parity")
	}
	require.NoError(t, os.WriteFile(file, fixed, 0600))
	window, err := handlers.LoadWindow(context.Background(), loader, base, "workflow", "conversation", nil)
	require.NoError(t, err)
	require.NotNil(t, window.View.Content)
}

package patch

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
)

func TestResolvePath_Workdir_NoBypass(t *testing.T) {
	base := string(filepath.Separator) + filepath.Join("workspace", "project")

	testCases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "relative path stays under workdir",
			path: "a/b.txt",
			want: filepath.Join(base, "a", "b.txt"),
		},
		{
			name: "absolute path is treated as relative (base name)",
			path: string(filepath.Separator) + filepath.Join("etc", "passwd"),
			want: filepath.Join(base, "passwd"),
		},
		{
			name: "path traversal is cleaned and cannot escape workdir",
			path: "../secrets.txt",
			want: filepath.Join(base, "secrets.txt"),
		},
		{
			name: "nested traversal is cleaned and cannot escape workdir",
			path: "a/../../secrets.txt",
			want: filepath.Join(base, "secrets.txt"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolvePath(tc.path, base)
			assert.Equal(t, tc.want, got)
			assert.True(t, strings.HasPrefix(got, base+string(filepath.Separator)) || got == base, "resolved path must stay under workdir")
		})
	}
}

func TestServiceApply_TraversalCantWriteOutsideWorkdir(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	workdir := "mem://localhost/work"
	outside := "mem://localhost/outside.txt"

	// Ensure workdir exists.
	assert.NoError(t, fs.Create(ctx, workdir, 0o755, true))

	s := New()

	// Attempt to write outside of workdir via traversal.
	patchText := `*** Begin Patch
*** Add File: ../outside.txt
+pwn
*** End Patch`

	var out ApplyOutput
	err := s.applyPatch(ctx, &ApplyInput{Patch: patchText, Workdir: workdir}, &out)
	assert.NoError(t, err)

	exists, err := fs.Exists(ctx, outside)
	assert.NoError(t, err)
	assert.False(t, exists, "should not be able to write outside the workdir")

	exists, err = fs.Exists(ctx, "mem://localhost/work/outside.txt")
	assert.NoError(t, err)
	assert.True(t, exists, "should have written inside workdir instead")
}

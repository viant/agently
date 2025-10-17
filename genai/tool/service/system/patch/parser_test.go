package patch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		name    string
		patch   string
		want    []Hunk
		wantErr bool
	}{
		{
			name: "add-delete-update",
			patch: `*** Begin Patch
*** Add File: path/add.py
+abc
+def
*** Delete File: path/delete.py
*** Update File: path/update.py
*** Move to: path/update2.py
@@ def f():
-    pass
+    return 123
*** End Patch`,
			want: []Hunk{
				AddFile{Path: "path/add.py", Contents: "abc\ndef\n"},
				DeleteFile{Path: "path/delete.py"},
				UpdateFile{
					Path:     "path/update.py",
					MovePath: "path/update2.py",
					Chunks: []UpdateChunk{
						{
							ChangeContext: "def f():",
							OldLines:      []string{"    pass"},
							NewLines:      []string{"    return 123"},
							IsEOF:         false,
						},
					},
				},
			},
		},
		{
			name: "empty",
			patch: `*** Begin Patch
*** End Patch`,
			want:    nil,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.patch)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, tc.want, got)
		})
	}
}

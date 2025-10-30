package patch_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	patch "github.com/viant/fluxor/service/action/system/patch"
)

// stripChangeDiffs returns a shallow copy of changes with Diff cleared to
// make comparisons stable regardless of patch formatting.
func stripChangeDiffs(in []patch.Change) []patch.Change {
	out := make([]patch.Change, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Diff = ""
	}
	return out
}

func TestSession_ApplyFailureKeepsSession_ForExplicitRollback(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name              string
		patchText         string
		expectedSnapshot  []patch.Change
		expectedExistence map[string]bool // after explicit rollback
	}

	testCases := []testCase{
		{
			name: "add then failing update leaves session active",
			patchText: `*** Begin Patch
*** Add File: mem://localhost/foo.txt
+hello
*** Update File: mem://localhost/bar.txt
@@
-aaa
+bbb
*** End Patch`,
			expectedSnapshot: []patch.Change{
				{Kind: "create", URL: "mem://localhost/foo.txt"},
			},
			expectedExistence: map[string]bool{
				"mem://localhost/foo.txt": false,
				"mem://localhost/bar.txt": false,
			},
		},
	}

	fs := afs.New()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session, err := patch.NewSession()
			if err != nil {
				t.Fatalf("new session: %v", err)
			}

			// Apply the patch. Expect an error due to the invalid update.
			err = session.ApplyPatch(ctx, tc.patchText)
			assert.NotNil(t, err, "apply should return an error")

			// Session should remain active; snapshot should include the successful add.
			got, snapErr := session.Snapshot(ctx)
			if snapErr != nil {
				t.Fatalf("snapshot: %v", snapErr)
			}
			assert.EqualValues(t, stripChangeDiffs(tc.expectedSnapshot), stripChangeDiffs(got))

			// Explicitly rollback and verify filesystem state matches expected existence.
			rbErr := session.Rollback(ctx)
			if rbErr != nil {
				t.Fatalf("rollback: %v", rbErr)
			}
			for path, want := range tc.expectedExistence {
				exists, _ := fs.Exists(ctx, path)
				assert.EqualValues(t, want, exists, "exists(%s)", path)
			}
		})
	}
}

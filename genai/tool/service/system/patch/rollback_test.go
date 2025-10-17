package patch_test

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	"github.com/viant/fluxor/service/action/system/patch"
	"strings"
	"testing"
)

func TestSession_Rollback(t *testing.T) {
	// Create a filesystem service
	fs := afs.New()
	ctx := context.Background()

	testCases := []struct {
		name          string
		operations    func(ctx context.Context, session *patch.Session) error
		initialFiles  map[string]string
		expectedFiles map[string]string // Expected files after rollback
	}{
		{
			name: "rollback add operation",
			operations: func(ctx context.Context, session *patch.Session) error {
				return session.Add(ctx, "mem://localhost/new_file.txt", []byte("This is a new file"))
			},
			initialFiles: map[string]string{},
			// After rollback, the added file should be removed
			expectedFiles: map[string]string{},
		},
		{
			name: "rollback update operation",
			operations: func(ctx context.Context, session *patch.Session) error {
				return session.Update(ctx, "mem://localhost/existing_file.txt", []byte("Updated content"))
			},
			initialFiles: map[string]string{
				"mem://localhost/existing_file.txt": "Original content",
			},
			// After rollback, the file should have its original content
			expectedFiles: map[string]string{
				"mem://localhost/existing_file.txt": "Original content",
			},
		},
		{
			name: "rollback delete operation",
			operations: func(ctx context.Context, session *patch.Session) error {
				return session.Delete(ctx, "mem://localhost/to_delete.txt")
			},
			initialFiles: map[string]string{
				"mem://localhost/to_delete.txt": "File to be deleted",
			},
			// After rollback, the deleted file should be restored
			expectedFiles: map[string]string{
				"mem://localhost/to_delete.txt": "File to be deleted",
			},
		},
		{
			name: "rollback move operation",
			operations: func(ctx context.Context, session *patch.Session) error {
				return session.Move(ctx, "mem://localhost/source.txt", "mem://localhost/destination.txt")
			},
			initialFiles: map[string]string{
				"mem://localhost/source.txt": "File to be moved",
			},
			// After rollback, the file should be back at its original location
			expectedFiles: map[string]string{
				"mem://localhost/source.txt": "File to be moved",
			},
		},
		{
			name: "rollback multiple operations",
			operations: func(ctx context.Context, session *patch.Session) error {
				if err := session.Add(ctx, "mem://localhost/new_file.txt", []byte("This is a new file")); err != nil {
					return err
				}
				if err := session.Update(ctx, "mem://localhost/existing_file.txt", []byte("Updated content")); err != nil {
					return err
				}
				if err := session.Delete(ctx, "mem://localhost/to_delete.txt"); err != nil {
					return err
				}
				if err := session.Move(ctx, "mem://localhost/source.txt", "mem://localhost/destination.txt"); err != nil {
					return err
				}
				return nil
			},
			initialFiles: map[string]string{
				"mem://localhost/existing_file.txt": "Original content",
				"mem://localhost/to_delete.txt":     "File to be deleted",
				"mem://localhost/source.txt":        "File to be moved",
			},
			// After rollback, all files should be in their original state
			expectedFiles: map[string]string{
				"mem://localhost/existing_file.txt": "Original content",
				"mem://localhost/to_delete.txt":     "File to be deleted",
				"mem://localhost/source.txt":        "File to be moved",
			},
		},
		{
			name: "rollback patch operations",
			operations: func(ctx context.Context, session *patch.Session) error {
				patchText := `*** Begin Patch
*** Update File: mem://localhost/existing_file.txt
@@ Original content
- Original content
+ Modified content
*** End Patch`
				return session.ApplyPatch(ctx, patchText)
			},
			initialFiles: map[string]string{
				"mem://localhost/existing_file.txt": "Original content",
			},
			// After rollback, the file should have its original content
			expectedFiles: map[string]string{
				"mem://localhost/existing_file.txt": "Original content",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup initial files
			for path, content := range tc.initialFiles {
				err := fs.Upload(ctx, path, 0644, strings.NewReader(content))
				if err != nil {
					t.Fatalf("Failed to create initial file %s: %v", path, err)
				}
			}

			// Create a new session
			session, err := patch.NewSession()
			if err != nil {
				t.Fatalf("Failed to create session: %v", err)
			}

			// Apply operations
			err = tc.operations(ctx, session)
			if err != nil {
				t.Fatalf("Failed to apply operations: %v", err)
			}

			// Rollback the session
			err = session.Rollback(ctx)
			if err != nil {
				t.Fatalf("Failed to rollback session: %v", err)
			}

			// Verify files after rollback
			for path, expectedContent := range tc.expectedFiles {
				exists, err := fs.Exists(ctx, path)
				if err != nil {
					t.Fatalf("Error checking if file exists: %v", err)
				}
				assert.True(t, exists, "File should exist after rollback: %s", path)

				data, err := fs.DownloadWithURL(ctx, path)
				if err != nil {
					t.Fatalf("Failed to read file after rollback: %v", err)
				}
				assert.Equal(t, expectedContent, string(data), "File content should match original after rollback: %s", path)
			}

			// Verify that added files were removed
			for path := range tc.initialFiles {
				if _, expected := tc.expectedFiles[path]; !expected {
					exists, _ := fs.Exists(ctx, path)
					assert.False(t, exists, "File should not exist after rollback: %s", path)
				}
			}

			// Clean up
			for path := range tc.initialFiles {
				_ = fs.Delete(ctx, path)
			}
			for path := range tc.expectedFiles {
				_ = fs.Delete(ctx, path)
			}
		})
	}
}

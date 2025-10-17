package patch_test

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	"github.com/viant/fluxor/service/action/system/patch"
	"strings"
	"testing"
)

// normalizeWhitespace removes all whitespace characters to make comparison whitespace insensitive
func normalizeWhitespace(s string) string {
	// Remove all whitespace characters (spaces, tabs, carriage returns)
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "\t", ""), "\r", "")
}

func TestService_ApplyPatch(t *testing.T) {

	var addFilePatch = `*** Begin Patch
*** Add File: mem://localhost/service_test.go
+package file_test
+
+import (
+    "bytes"
+    "context"
+    "fmt"
+    "io/ioutil"
+    "os"
+    "path/filepath"
+    "testing"
+
+    ffile "github.com/viant/forge/backend/service/file"
+    "github.com/viant/afs/storage"
+)
+
+// setupTempFS creates a temporary directory with files and folders for testing
*** End Patch`

	testCases := []struct {
		name          string
		patches       []string
		existingFiles map[string]string
		expectedFiles map[string]string // path -> content
	}{

		{
			name: "updated existing file",
			existingFiles: map[string]string{
				"mem://localhost/service_test.go": `package file_test

// New creates a new Service.
func New(root string, options ...storage.Option) *Service {
	return &Service{
		root:    root,
		options: options,
		service: afs.New(), // this creates a new default AFS service
	}
}

// List returns the files and directories at requestedPath (relative to Service.root).
func (f *Service) List(ctx context.Context, opts ...Option) ([]File, error) {
	// Build the full path by combining the root and the requested path.
	options := newOptions(opts...)
	uri := options.uri

	// Check if the path actually exists.
	exists, _ := f.service.Exists(ctx, URL)
	if !exists {
		return nil, fmt.Errorf("path %q does not exist", URL)
	}

   objects, err := f.service.List(context.Background(), URL)
   if err != nil {
       return nil, err
   }

	var items []File

   for _, obj := range objects {
       if url.Equals(URL, obj.URL()) {
           continue
       }
	   if options.onlyFolder && !obj.IsDir() {
            continue
	   }
	}
}
`,
			},
			patches: []string{
				`*** Begin Patch
*** Update File: mem://localhost/service_test.go
@@ func (f *Service) List(ctx context.Context, opts ...Option) ([]File, error) {
-   objects, err := f.service.List(context.Background(), URL)
+   objects, err := f.service.List(context.Background(), URL1)
@@
-   for _, obj := range objects {
-       if url.Equals(URL, obj.URL()) {
-           continue
-       }
+   for _, obj := range objects {
+       // Skip the parent directory itself
+       if url.Equals(parentURL, obj.URL()) {
+           continue
+       }
*** End Patch
`,
			},
			expectedFiles: map[string]string{
				"mem://localhost/service_test.go": `package file_test

// New creates a new Service.
func New(root string, options ...storage.Option) *Service {
	return &Service{
		root:    root,
		options: options,
		service: afs.New(), // this creates a new default AFS service
	}
}

// List returns the files and directories at requestedPath (relative to Service.root).
func (f *Service) List(ctx context.Context, opts ...Option) ([]File, error) {
	// Build the full path by combining the root and the requested path.
	options := newOptions(opts...)
	uri := options.uri

	// Check if the path actually exists.
	exists, _ := f.service.Exists(ctx, URL)
	if !exists {
		return nil, fmt.Errorf("path %q does not exist", URL)
	}

   objects, err := f.service.List(context.Background(), URL1)
   if err != nil {
       return nil, err
   }

	var items []File

   for _, obj := range objects {
       // Skip the parent directory itself
       if url.Equals(parentURL, obj.URL()) {
           continue
       }
	   if options.onlyFolder && !obj.IsDir() {
            continue
	   }
	}
}
`,
			},
		},
		{
			name: "single patch - add file",
			patches: []string{
				addFilePatch,
			},
			expectedFiles: map[string]string{
				"mem://localhost/service_test.go": `package file_test

import (
    "bytes"
    "context"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "testing"

    ffile "github.com/viant/forge/backend/service/file"
    "github.com/viant/afs/storage"
)

// setupTempFS creates a temporary directory with files and folders for testing
`,
			},
		},
		{
			name: "single line addition \"github.com/viant/afs/option\"",
			patches: []string{
				addFilePatch,
				`*** Begin Patch
*** Update File: mem://localhost/service_test.go
@@ import (
-   ffile "github.com/viant/forge/backend/service/file"
-   "github.com/viant/afs/storage"
+   ffile "github.com/viant/forge/backend/service/file"
+   "github.com/viant/afs/storage"
+   "github.com/viant/afs/option"
*** End Patch`,
			},
			expectedFiles: map[string]string{
				"mem://localhost/service_test.go": `package file_test

import (
    "bytes"
    "context"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "testing"

    ffile "github.com/viant/forge/backend/service/file"
    "github.com/viant/afs/storage"
   "github.com/viant/afs/option"
)

// setupTempFS creates a temporary directory with files and folders for testing
`,
			},
		},

		{
			name: "multi line removal",
			patches: []string{
				addFilePatch,
				`*** Begin Patch
*** Update File: mem://localhost/service_test.go
@@ import (
-   ffile "github.com/viant/forge/backend/service/file"
-   "github.com/viant/afs/storage"
*** End Patch`,
			},
			expectedFiles: map[string]string{
				"mem://localhost/service_test.go": `package file_test

import (
    "bytes"
    "context"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "testing"

)

// setupTempFS creates a temporary directory with files and folders for testing
`,
			},
		},
	}
	fs := afs.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for this test case

			// Create a new patch session
			session, err := patch.NewSession()
			if err != nil {
				t.Fatalf("failed to create patch session: %v", err)
			}

			// Create context
			ctx := context.Background()

			for existingFile, content := range tc.existingFiles {
				err = fs.Upload(ctx, existingFile, 0777, strings.NewReader(content))
				if !assert.Nil(t, err, "failed to create file: %v", err) {
					continue
				}
			}
			// Apply each patch
			for i, patchText := range tc.patches {
				err = session.ApplyPatch(ctx, patchText)
				if err != nil {
					t.Fatalf("failed to apply patch %d: %v", i, err)
				}
			}

			// Commit the changes
			if err := session.Commit(ctx); err != nil {
				t.Fatalf("failed to commit changes: %v", err)
			}

			// Verify the final state of the files
			for path, expectedContent := range tc.expectedFiles {

				// Read file content
				data, err := fs.DownloadWithURL(context.TODO(), path)
				if err != nil {
					t.Fatalf("failed to read file: %v", err)
				}

				actualContent := string(data)
				// Normalize content by trimming trailing newlines and making whitespace insensitive
				expectedNormalized := strings.TrimRight(expectedContent, "\n")
				actualNormalized := strings.TrimRight(actualContent, "\n")

				// For assertion purposes, we'll compare normalized versions (whitespace insensitive)
				// but still show the original content in case of failure
				expectedForComparison := normalizeWhitespace(expectedNormalized)
				actualForComparison := normalizeWhitespace(actualNormalized)

				if expectedForComparison != actualForComparison {
					// Use assert.Equal to get the nice diff output, but with original content
					fmt.Printf("file content mismatch for %s\n", path)
					assert.Equal(t, expectedNormalized, actualNormalized, "file content mismatch for %s", path)
				}
			}
		})
	}
}

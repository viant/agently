package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	agmodel "github.com/viant/agently/genai/agent"
	aug "github.com/viant/agently/genai/service/augmenter"
)

// dummyAugmenter is used only to satisfy the Service constructor in tests that
// do not exercise the match method.
func dummyAugmenter(t *testing.T) *aug.Service {
	t.Helper()
	// nil finder is acceptable as long as we do not call match.
	return aug.New(nil)
}

func TestService_ListAndRead_LocalRoot(t *testing.T) {
	t.Run("list and read under workspace root", func(t *testing.T) {
		fs := afs.New()
		// Create a folder under workspace root
		// Use a stable subfolder to avoid relying on env
		base := ".agently/test_resources"
		// Ensure directory exists
		_ = os.MkdirAll(base, 0755)
		filePath := filepath.Join(base, "sample.txt")
		content := []byte("hello resources")
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_ = fs

		service := New(dummyAugmenter(t))

		rootURI := "workspace://localhost/test_resources"

		// List
		listInput := &ListInput{
			RootURI:   rootURI,
			Recursive: false,
			MaxItems:  10,
		}
		listOutput := &ListOutput{}
		ctx := context.Background()
		if err := service.list(ctx, listInput, listOutput); err != nil {
			t.Fatalf("list returned error: %v", err)
		}
		if assert.Len(t, listOutput.Items, 1) {
			item := listOutput.Items[0]
			assert.EqualValues(t, "sample.txt", item.Name)
			assert.EqualValues(t, "sample.txt", item.Path)
			assert.EqualValues(t, int64(len(content)), item.Size)
			assert.WithinDuration(t, time.Now(), item.Modified, time.Minute)
		}

		// Read
		readInput := &ReadInput{RootURI: rootURI, Path: "sample.txt"}
		readOutput := &ReadOutput{}
		if err := service.read(ctx, readInput, readOutput); err != nil {
			t.Fatalf("read returned error: %v", err)
		}
		assert.EqualValues(t, "sample.txt", readOutput.Path)
		assert.EqualValues(t, string(content), readOutput.Content)
		assert.EqualValues(t, len(content), readOutput.Size)
	})

	t.Run("read by workspace uri only", func(t *testing.T) {
		fs := afs.New()
		base := ".agently/test_resources_uri"
		_ = os.MkdirAll(base, 0755)
		filePath := filepath.Join(base, "sample.txt")
		content := []byte("hello by uri")
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_ = fs
		service := New(dummyAugmenter(t))
		ctx := context.Background()

		uri := "workspace://localhost/test_resources_uri/sample.txt"
		readInput := &ReadInput{URI: uri}
		readOutput := &ReadOutput{}
		if err := service.read(ctx, readInput, readOutput); err != nil {
			t.Fatalf("read returned error: %v", err)
		}
		assert.EqualValues(t, workspaceToFile(uri), readOutput.URI)
		assert.EqualValues(t, string(content), readOutput.Content)
		assert.EqualValues(t, len(content), readOutput.Size)
	})

	// No range slicing: always return full content
	t.Run("read returns full content", func(t *testing.T) {
		base := ".agently/test_resources_full"
		_ = os.MkdirAll(base, 0755)
		filePath := filepath.Join(base, "sample.txt")
		content := []byte("abcdefghijklmnopqrstuvwxyz")
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		service := New(dummyAugmenter(t))
		ctx := context.Background()
		rootURI := "workspace://localhost/test_resources_full"
		readInput := &ReadInput{RootURI: rootURI, Path: "sample.txt"}
		readOutput := &ReadOutput{}
		if err := service.read(ctx, readInput, readOutput); err != nil {
			t.Fatalf("read returned error: %v", err)
		}
		assert.EqualValues(t, "sample.txt", readOutput.Path)
		assert.EqualValues(t, string(content), readOutput.Content)
		assert.EqualValues(t, len(content), readOutput.Size)
		assert.EqualValues(t, 0, readOutput.StartLine)
		assert.EqualValues(t, 0, readOutput.EndLine)
	})
}

func TestService_GrepFiles_LocalRoot(t *testing.T) {
	fs := afs.New()
	_ = fs
	base := ".agently/test_resources_grep"
	_ = os.MkdirAll(base, 0755)
	// Create a couple of files
	files := map[string]string{
		"a.txt": "hello world\nAuthMode here\n",
		"b.txt": "no match here\n",
		"c.log": "AuthMode again\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(base, name), []byte(body), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", name, err)
		}
	}

	service := New(dummyAugmenter(t))
	ctx := context.Background()
	rootURI := "workspace://localhost/test_resources_grep"

	t.Run("basic grepFiles by pattern", func(t *testing.T) {
		in := &GrepInput{
			Pattern:   "AuthMode",
			RootURI:   rootURI,
			Path:      ".",
			Recursive: true,
			Include:   []string{"*.txt", "*.log"},
		}
		out := &GrepOutput{}
		if err := service.grepFiles(ctx, in, out); err != nil {
			t.Fatalf("grepFiles returned error: %v", err)
		}
		// We expect matches in a.txt and c.log
		assert.GreaterOrEqual(t, out.Stats.Matched, 2)
		paths := map[string]bool{}
		for _, f := range out.Files {
			paths[f.Path] = true
		}
		assert.True(t, paths["a.txt"])
		assert.True(t, paths["c.log"])
	})

	t.Run("pattern must not be empty", func(t *testing.T) {
		in := &GrepInput{Pattern: "   ", RootURI: rootURI, Path: "."}
		out := &GrepOutput{}
		err := service.grepFiles(ctx, in, out)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "pattern must not be empty")
		}
	})
}

func TestResourceFlags_SemanticAndGrepAllowed(t *testing.T) {
	ag := &agmodel.Agent{
		Resources: []*agmodel.Resource{
			{
				URI: "workspace://localhost/agents/foo",
				// explicit disable semantic, allow grep
				AllowSemanticMatch: func() *bool { b := false; return &b }(),
				AllowGrep:          func() *bool { b := true; return &b }(),
			},
			{
				URI: "workspace://localhost/agents/bar",
				// default (nil) flags -> both allowed
			},
		},
	}
	service := &Service{}
	ctx := context.Background()

	// Semantic match disabled on foo, enabled on bar and others
	assert.False(t, service.semanticAllowedForAgent(ctx, ag, "workspace://localhost/agents/foo"))
	assert.True(t, service.semanticAllowedForAgent(ctx, ag, "workspace://localhost/agents/bar"))
	assert.True(t, service.semanticAllowedForAgent(ctx, ag, "workspace://localhost/other"))

	// Grep allowed on foo (explicit true), on bar (default), and on others
	assert.True(t, service.grepAllowedForAgent(ctx, ag, "workspace://localhost/agents/foo"))
	assert.True(t, service.grepAllowedForAgent(ctx, ag, "workspace://localhost/agents/bar"))
	assert.True(t, service.grepAllowedForAgent(ctx, ag, "workspace://localhost/other"))
}

func TestSplitPatterns_AndCompilePatterns(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect []string
	}{
		{"single", "AuthMode", []string{"AuthMode"}},
		{"pipe", "AuthMode|TokenData", []string{"AuthMode", "TokenData"}},
		{"or lowercase", "AuthMode or TokenData", []string{"AuthMode", "TokenData"}},
		{"or uppercase", "AuthMode OR TokenData", []string{"AuthMode", "TokenData"}},
		{"mixed", " AuthMode | TokenData or Foo ", []string{"AuthMode", "TokenData", "Foo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitPatterns(tc.input)
			assert.EqualValues(t, tc.expect, got)
		})
	}

	// compilePatterns should respect caseInsensitive flag
	patterns := splitPatterns("AuthMode|TokenData")
	reInsensitive, err := compilePatterns(patterns, true)
	if err != nil {
		t.Fatalf("compilePatterns case-insensitive error: %v", err)
	}
	reSensitive, err := compilePatterns(patterns, false)
	if err != nil {
		t.Fatalf("compilePatterns case-sensitive error: %v", err)
	}
	lineLower := "authmode appears here"
	lineUpper := "AuthMode appears here"
	// In case-insensitive mode, both lines should match
	assert.True(t, lineMatches(lineLower, reInsensitive, nil))
	assert.True(t, lineMatches(lineUpper, reInsensitive, nil))
	// In case-sensitive mode, only the exact-case line should match
	assert.False(t, lineMatches(lineLower, reSensitive, nil))
	assert.True(t, lineMatches(lineUpper, reSensitive, nil))
}

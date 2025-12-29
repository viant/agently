package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viant/afs"
	agmodel "github.com/viant/agently/genai/agent"
	aug "github.com/viant/agently/genai/service/augmenter"
	embSchema "github.com/viant/embedius/schema"
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
		if assert.GreaterOrEqual(t, len(listOutput.Items), 1, "expected at least one item under workspace root") {
			var item *ListItem
			for i := range listOutput.Items {
				if listOutput.Items[i].Name == "sample.txt" {
					item = &listOutput.Items[i]
					break
				}
			}
			if item == nil {
				t.Fatalf("expected to find sample.txt in list output, got: %+v", listOutput.Items)
			}
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

	type testCase struct {
		name      string
		input     *GrepInput
		expect    []string
		expectMin int
	}
	testCases := []testCase{
		{
			name: "basic grepFiles by pattern",
			input: &GrepInput{
				Pattern:   "AuthMode",
				RootURI:   rootURI,
				Path:      ".",
				Recursive: true,
				Include:   []string{"*.txt", "*.log"},
			},
			expect:    []string{"a.txt", "c.log"},
			expectMin: 2,
		},
		{
			name: "path points to file",
			input: &GrepInput{
				Pattern:   "AuthMode",
				RootURI:   rootURI,
				Path:      "a.txt",
				Recursive: false,
			},
			expect:    []string{"a.txt"},
			expectMin: 1,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := &GrepOutput{}
			if err := service.grepFiles(ctx, tc.input, out); err != nil {
				t.Fatalf("grepFiles returned error: %v", err)
			}
			assert.EqualValues(t, false, out.Stats.Truncated)
			assert.EqualValues(t, true, out.Stats.Matched >= tc.expectMin)
			paths := map[string]bool{}
			for _, f := range out.Files {
				paths[f.Path] = true
			}
			for _, expected := range tc.expect {
				assert.EqualValues(t, true, paths[expected])
			}
		})
	}

	t.Run("pattern must not be empty", func(t *testing.T) {
		in := &GrepInput{Pattern: "   ", RootURI: rootURI, Path: "."}
		out := &GrepOutput{}
		err := service.grepFiles(ctx, in, out)
		assert.EqualValues(t, true, err != nil)
		if err != nil {
			assert.EqualValues(t, true, strings.Contains(err.Error(), "pattern must not be empty"))
		}
	})
}

func TestService_ReadImage(t *testing.T) {
	base := ".agently/test_resources_image"
	require.NoError(t, os.MkdirAll(base, 0o755))

	img := image.NewRGBA(image.Rect(0, 0, 10, 5))
	imgPath := filepath.Join(base, "img.png")
	f, err := os.Create(imgPath)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())

	service := New(dummyAugmenter(t))
	ctx := context.Background()
	rootURI := "workspace://localhost/test_resources_image"

	type testCase struct {
		name      string
		input     *ReadImageInput
		expectErr bool
		assertFn  func(t *testing.T, out *ReadImageOutput, err error)
	}

	testCases := []testCase{
		{
			name: "reads and returns base64 when includeData",
			input: &ReadImageInput{
				RootURI:     rootURI,
				Path:        "img.png",
				IncludeData: true,
				MaxWidth:    256,
				MaxHeight:   256,
				MaxBytes:    1024 * 1024,
				Format:      "png",
				DestURL:     "",
			},
			expectErr: false,
			assertFn: func(t *testing.T, out *ReadImageOutput, err error) {
				assert.EqualValues(t, nil, err)
				assert.EqualValues(t, "img.png", out.Path)
				assert.EqualValues(t, "img.png", out.Name)
				assert.EqualValues(t, "image/png", out.MimeType)
				assert.EqualValues(t, true, strings.HasPrefix(out.Encoded, "file://"))
				assert.EqualValues(t, true, out.Width > 0)
				assert.EqualValues(t, true, out.Height > 0)
				raw, decErr := base64.StdEncoding.DecodeString(out.Base64)
				assert.EqualValues(t, nil, decErr)
				assert.EqualValues(t, out.Bytes, len(raw))
			},
		},
		{
			name: "reads without base64 by default",
			input: &ReadImageInput{
				RootURI: rootURI,
				Path:    "img.png",
			},
			expectErr: false,
			assertFn: func(t *testing.T, out *ReadImageOutput, err error) {
				assert.EqualValues(t, nil, err)
				assert.EqualValues(t, "", out.Base64)
				assert.EqualValues(t, true, strings.HasPrefix(out.Encoded, "file://"))
				assert.EqualValues(t, true, out.Bytes > 0)
			},
		},
		{
			name: "rejects empty",
			input: &ReadImageInput{
				RootURI: rootURI,
				Path:    "",
			},
			expectErr: true,
			assertFn: func(t *testing.T, _ *ReadImageOutput, err error) {
				assert.EqualValues(t, true, err != nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := &ReadImageOutput{}
			err := service.readImage(ctx, tc.input, out)
			if tc.expectErr {
				assert.EqualValues(t, true, err != nil)
			}
			if tc.assertFn != nil {
				tc.assertFn(t, out, err)
			}
		})
	}

	// Ensure JSON marshaling uses the expected keys for tool pipeline decoding.
	out := &ReadImageOutput{URI: "u", Encoded: "file:///tmp/u.png", Path: "p", Name: "n", MimeType: "image/png", Width: 1, Height: 1, Bytes: 1, Base64: "AA=="}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	assert.EqualValues(t, true, strings.Contains(string(data), "\"dataBase64\""))
	assert.EqualValues(t, true, strings.Contains(string(data), "\"mimeType\""))
	assert.EqualValues(t, true, strings.Contains(string(data), "\"encodedURI\""))
}

func TestSelectSearchRoots_InvalidRootID(t *testing.T) {
	service := New(dummyAugmenter(t))
	roots := []Root{
		{ID: "bidder", URI: "workspace://localhost/knowledge/bidder", AllowedSemanticSearch: true},
		{ID: "mdp", URI: "workspace://localhost/knowledge/mdp", AllowedSemanticSearch: true},
	}
	input := &MatchInput{RootIDs: []string{"workspace://localhost/knowledge"}}
	_, err := service.selectSearchRoots(context.Background(), roots, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown rootId")
}

func TestSelectSearchRoots_FallbackToURI(t *testing.T) {
	service := New(dummyAugmenter(t))
	roots := []Root{
		{ID: "bidder", URI: "workspace://localhost/knowledge/bidder", AllowedSemanticSearch: true},
		{ID: "mdp", URI: "workspace://localhost/knowledge/mdp", AllowedSemanticSearch: true},
	}
	input := &MatchInput{RootIDs: []string{"workspace://localhost/knowledge/bidder"}}
	selected, err := service.selectSearchRoots(context.Background(), roots, input)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.EqualValues(t, "bidder", selected[0].ID)
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

func TestBuildDocumentContent_SystemOnly(t *testing.T) {
	docs := []embSchema.Document{
		{Metadata: map[string]any{"path": "workspace://localhost/sys/doc.txt"}, PageContent: "alpha"},
		{Metadata: map[string]any{"path": "workspace://localhost/user/notes.txt"}, PageContent: "beta"},
	}
	systemDocs := filterSystemDocuments(docs, []string{"workspace://localhost/sys"})
	got := buildDocumentContent(systemDocs, "workspace://localhost/")
	expect := "file: sys/doc.txt\n```txt\nalpha\n````\n\n"
	assert.Equal(t, expect, got)
}

func TestService_MatchDocuments(t *testing.T) {
	service := New(dummyAugmenter(t))
	service.defaultEmbedder = "dummy"
	service.defaults.Locations = []string{"workspace://localhost/sys", "workspace://localhost/user"}
	service.augmentDocsOverride = func(ctx context.Context, in *aug.AugmentDocsInput, out *aug.AugmentDocsOutput) error {
		out.Documents = []embSchema.Document{
			{Metadata: map[string]any{"path": "workspace://localhost/sys/doc.md"}, Score: 0.9},
			{Metadata: map[string]any{"path": "workspace://localhost/sys/doc.md"}, Score: 0.3},
			{Metadata: map[string]any{"path": "workspace://localhost/user/notes.md"}, Score: 0.4},
		}
		return nil
	}
	ctx := context.Background()
	input := &MatchDocumentsInput{
		Query:   "performance playbook",
		RootIDs: []string{"workspace://localhost/sys", "workspace://localhost/user"},
		Model:   "dummy",
	}
	output := &MatchDocumentsOutput{}
	require.NoError(t, service.matchDocuments(ctx, input, output))
	require.Len(t, output.Documents, 2)
	assert.Equal(t, "workspace://localhost/sys/doc.md", output.Documents[0].URI)
	assert.Equal(t, "workspace://localhost/sys", output.Documents[0].RootID)
	assert.InDelta(t, 0.9, output.Documents[0].Score, 0.001)
	assert.Equal(t, "workspace://localhost/user/notes.md", output.Documents[1].URI)
	assert.Equal(t, "workspace://localhost/user", output.Documents[1].RootID)
}

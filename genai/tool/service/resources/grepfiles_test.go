package resources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	agmodel "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/hygine"
	"github.com/viant/agently/genai/memory"
	convmem "github.com/viant/agently/internal/service/conversation/memory"
)

func writeFileFS(t *testing.T, rootFS, rel string, content []byte) {
	t.Helper()
	full := filepath.Join(rootFS, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, content, 0o644))
}

func makeWorkspaceRoot(t *testing.T, rel string) (rootURI string, rootFS string) {
	t.Helper()
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "/"))
	rootURI = "workspace://localhost/" + rel
	rootFS = filepath.Join(".agently", filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(rootFS, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(rootFS) })
	return rootURI, rootFS
}

func createGrepFixture(t *testing.T, rootFS string) {
	t.Helper()
	writeFileFS(t, rootFS, "fields.go", []byte("package x\n\nimport \"fmt\"\n\n// AuthMode keep\n"))
	writeFileFS(t, rootFS, "sub/fields.go", []byte("package y\nimport \"os\"\n// Token keep\n"))
	writeFileFS(t, rootFS, "sub/inner/deep.go", []byte("package z\nimport \"strings\"\n// AUTHMODE upper\n"))
	writeFileFS(t, rootFS, "sub/inner/deep.log", []byte("AuthMode deep log\n"))
	writeFileFS(t, rootFS, "a.txt", []byte("hello world\nAuthMode here\nToken here\n"))
	writeFileFS(t, rootFS, "b.txt", []byte("no match here\n"))
	writeFileFS(t, rootFS, "c.log", []byte("AuthMode again\n"))
	writeFileFS(t, rootFS, "mixed.txt", []byte("AuthMode again\nAuthMode keep\n"))
	writeFileFS(t, rootFS, "blocks.txt", []byte("import block 1\nimport block 2\nimport block 3\nimport block 4\n"))
	writeFileFS(t, rootFS, "longline.go", []byte("import \""+strings.Repeat("a", 512)+"\"\n"))
	writeFileFS(t, rootFS, "binary.bin", []byte{0x00, 0x01, 'i', 'm', 'p', 'o', 'r', 't', 0x00})
	// Match appears only after ~1500 bytes to validate MaxSize clipping.
	writeFileFS(t, rootFS, "big.txt", []byte(strings.Repeat("x", 1500)+"import\n"))
}

func pathsSet(files []hygine.GrepFile) map[string]bool {
	out := make(map[string]bool, len(files))
	for _, f := range files {
		out[f.Path] = true
	}
	return out
}

func TestGrepFiles_RootURI(t *testing.T) {
	service := New(dummyAugmenter(t))
	ctx := context.Background()

	t.Run("workspace root uri", func(t *testing.T) {
		rootURI, rootFS := makeWorkspaceRoot(t, "test_grepfiles/rooturi_ws")
		createGrepFixture(t, rootFS)

		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/fields.go"},
			Mode:      "match",
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("file root uri", func(t *testing.T) {
		rootURI := tempDirURL(t)
		rootFS := strings.TrimPrefix(rootURI, "file://")
		createGrepFixture(t, rootFS)

		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/fields.go"},
			Mode:      "match",
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("relative root uri is treated as workspace root", func(t *testing.T) {
		rel := "test_grepfiles/rooturi_rel"
		_, rootFS := makeWorkspaceRoot(t, rel)
		createGrepFixture(t, rootFS)

		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rel,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("mcp root uri is rejected", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern: "import",
			RootURI: "mcp:server:/repo",
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "grepFiles is not supported for mcp resources")
	})

	t.Run("root or rootId required", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern: "import",
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "root or rootId is required")
	})
}

func TestGrepFiles_RootID_ResolutionAndPermissions(t *testing.T) {
	agentID := "test-agent"
	convID := "conv-1"
	convClient := convmem.New()
	conv := apiconv.NewConversation()
	conv.SetId(convID)
	conv.SetAgentId(agentID)
	require.NoError(t, convClient.PatchConversations(context.Background(), conv))

	fileRoot := tempDirURL(t)
	fileRootFS := strings.TrimPrefix(fileRoot, "file://")
	createGrepFixture(t, fileRootFS)

	wsRootIDURI, wsRootFS := makeWorkspaceRoot(t, "test_grepfiles/rootid_ws")
	_ = wsRootIDURI // workspace root uri is derived from resource URI below
	createGrepFixture(t, wsRootFS)

	noGrepRoot := tempDirURL(t)
	noGrepRootFS := strings.TrimPrefix(noGrepRoot, "file://")
	createGrepFixture(t, noGrepRootFS)

	agentsRootFS := filepath.Join(".agently", "agents", "_test_grepfiles_rootid")
	require.NoError(t, os.MkdirAll(agentsRootFS, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(agentsRootFS) })
	createGrepFixture(t, agentsRootFS)

	imagesRootFS := filepath.Join(".agently", "knowledge", "images", "_test_grepfiles_rootid")
	require.NoError(t, os.MkdirAll(imagesRootFS, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(imagesRootFS) })
	createGrepFixture(t, imagesRootFS)

	deny := false
	svc := New(dummyAugmenter(t),
		WithConversationClient(convClient),
		WithAgentFinder(&testAgentFinder{agent: &agmodel.Agent{
			Identity: agmodel.Identity{ID: agentID},
			Resources: []*agmodel.Resource{
				{ID: "local", URI: fileRoot, Role: "user"},
				{ID: "ws", URI: "test_grepfiles/rootid_ws", Role: "user"},
				{ID: "agents", URI: "agents", Role: "user"},
				{ID: "images", URI: "knowledge/images", Role: "user"},
				{ID: "nogrep", URI: noGrepRoot, Role: "user", AllowGrep: &deny},
				{ID: "mcp", URI: "mcp:server:/repo", Role: "user"},
			},
		}}),
	)
	ctx := memory.WithConversationID(context.Background(), convID)

	t.Run("case-insensitive rootId resolves", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootID:    "LOCAL",
			Path:      "",
			Recursive: true,
			Include:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		assert.True(t, pathsSet(out.Files)["fields.go"])
	})

	t.Run("workspace-relative resource uri resolves", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootID:    "ws",
			Path:      "",
			Recursive: true,
			Include:   []string{"**/fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("agents kind rootId works with sub-path", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootID:    "agents",
			Path:      "_test_grepfiles_rootid",
			Recursive: true,
			Include:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		assert.True(t, pathsSet(out.Files)["fields.go"])
	})

	t.Run("knowledge/images kind rootId works with sub-path", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootID:    "images",
			Path:      "_test_grepfiles_rootid",
			Recursive: true,
			Include:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		assert.True(t, pathsSet(out.Files)["fields.go"])
	})

	t.Run("allowGrep=false blocks grepFiles", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern: "import",
			RootID:  "nogrep",
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "grep not allowed")
	})

	t.Run("root allowlist blocks non-declared roots", func(t *testing.T) {
		other := tempDirURL(t)
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern: "import",
			RootURI: other,
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "root not allowed")
	})

	t.Run("unknown rootId returns error", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern: "import",
			RootID:  "unknown",
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown rootId")
	})

	t.Run("mcp rootId resolves but grepFiles rejects mcp roots", func(t *testing.T) {
		out := &GrepOutput{}
		err := svc.grepFiles(ctx, &GrepInput{
			Pattern: "import",
			RootID:  "mcp",
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "grepFiles is not supported for mcp resources")
	})

	t.Run("rootId-as-uri fallback works without agent context", func(t *testing.T) {
		svc2 := New(dummyAugmenter(t))

		out := &GrepOutput{}
		err := svc2.grepFiles(context.Background(), &GrepInput{
			Pattern:   "import",
			RootID:    fileRoot,
			Path:      "",
			Recursive: true,
			Include:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		assert.True(t, pathsSet(out.Files)["fields.go"])
	})

	t.Run("human-friendly rootId without agent context fails", func(t *testing.T) {
		svc2 := New(dummyAugmenter(t))

		out := &GrepOutput{}
		err := svc2.grepFiles(context.Background(), &GrepInput{
			Pattern: "import",
			RootID:  "local",
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown rootId")
	})
}

func TestGrepFiles_PathForms(t *testing.T) {
	service := New(dummyAugmenter(t))
	ctx := context.Background()

	type rootCase struct {
		name    string
		rootURI string
		rootFS  string
	}
	fileRootURI := tempDirURL(t)
	fileRootFS := strings.TrimPrefix(fileRootURI, "file://")
	createGrepFixture(t, fileRootFS)

	wsRootURI, wsRootFS := makeWorkspaceRoot(t, "test_grepfiles/pathforms_ws")
	createGrepFixture(t, wsRootFS)

	roots := []rootCase{
		{name: "file", rootURI: fileRootURI, rootFS: fileRootFS},
		{name: "workspace", rootURI: wsRootURI, rootFS: wsRootFS},
	}

	for _, rt := range roots {
		rt := rt
		t.Run(rt.name, func(t *testing.T) {
			t.Run("path empty walks root", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern:   "import",
					RootURI:   rt.rootURI,
					Path:      "",
					Recursive: true,
					Include:   []string{"**/fields.go"},
				}, out)
				require.NoError(t, err)
				got := pathsSet(out.Files)
				assert.True(t, got["fields.go"])
				assert.True(t, got["sub/fields.go"])
			})

			t.Run("path points to file uses fast path", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern: "import",
					RootURI: rt.rootURI,
					Path:    "fields.go",
				}, out)
				require.NoError(t, err)
				require.Len(t, out.Files, 1)
				assert.Equal(t, "fields.go", out.Files[0].Path)
			})

			t.Run("path points to subdir changes returned relative paths", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern:   "import",
					RootURI:   rt.rootURI,
					Path:      "sub",
					Recursive: false,
					Include:   []string{"**/*.go"},
				}, out)
				require.NoError(t, err)
				got := pathsSet(out.Files)
				assert.True(t, got["fields.go"])
				assert.False(t, got["inner/deep.go"])
			})

			t.Run("path slash is treated as root", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern:   "import",
					RootURI:   rt.rootURI,
					Path:      "/",
					Recursive: true,
					Include:   []string{"**/fields.go"},
				}, out)
				require.NoError(t, err)
				got := pathsSet(out.Files)
				assert.True(t, got["fields.go"])
				assert.True(t, got["sub/fields.go"])
			})

			t.Run("path dot is accepted", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern:   "import",
					RootURI:   rt.rootURI,
					Path:      ".",
					Recursive: true,
					Include:   []string{"**/fields.go"},
				}, out)
				require.NoError(t, err)
				got := pathsSet(out.Files)
				assert.True(t, got["fields.go"])
				assert.True(t, got["sub/fields.go"])
			})

			t.Run("absolute filesystem path under root is accepted", func(t *testing.T) {
				absBase, err := filepath.Abs(rt.rootFS)
				require.NoError(t, err)

				out := &GrepOutput{}
				err = service.grepFiles(ctx, &GrepInput{
					Pattern:   "import",
					RootURI:   rt.rootURI,
					Path:      filepath.Join(absBase, "sub"),
					Recursive: false,
					Include:   []string{"**/*.go"},
				}, out)
				require.NoError(t, err)
				assert.True(t, pathsSet(out.Files)["fields.go"])
			})

			t.Run("file:// url path under root is accepted", func(t *testing.T) {
				absBase, err := filepath.Abs(rt.rootFS)
				require.NoError(t, err)

				out := &GrepOutput{}
				err = service.grepFiles(ctx, &GrepInput{
					Pattern:   "import",
					RootURI:   rt.rootURI,
					Path:      "file://" + filepath.ToSlash(filepath.Join(absBase, "sub")),
					Recursive: false,
					Include:   []string{"**/*.go"},
				}, out)
				require.NoError(t, err)
				got := pathsSet(out.Files)
				assert.True(t, got["fields.go"])
				assert.False(t, got["inner/deep.go"])
			})

			t.Run("file:// url to file under root uses fast path", func(t *testing.T) {
				absBase, err := filepath.Abs(rt.rootFS)
				require.NoError(t, err)

				out := &GrepOutput{}
				err = service.grepFiles(ctx, &GrepInput{
					Pattern: "import",
					RootURI: rt.rootURI,
					Path:    "file://" + filepath.ToSlash(filepath.Join(absBase, "fields.go")),
				}, out)
				require.NoError(t, err)
				require.Len(t, out.Files, 1)
				assert.Equal(t, "fields.go", out.Files[0].Path)
			})

			t.Run("absolute filesystem path outside root is rejected", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern: "import",
					RootURI: rt.rootURI,
					Path:    t.TempDir(),
				}, out)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "outside root")
			})

			t.Run("workspace:// path is rejected as outside root", func(t *testing.T) {
				out := &GrepOutput{}
				err := service.grepFiles(ctx, &GrepInput{
					Pattern: "import",
					RootURI: rt.rootURI,
					Path:    "workspace://localhost/somewhere",
				}, out)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "outside root")
			})
		})
	}
}

func TestGrepFiles_IncludeExclude(t *testing.T) {
	service := New(dummyAugmenter(t))
	ctx := context.Background()

	rootURI := tempDirURL(t)
	rootFS := strings.TrimPrefix(rootURI, "file://")
	createGrepFixture(t, rootFS)

	t.Run("include empty means all files eligible", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/inner/deep.go"])
	})

	t.Run("include by basename matches any depth (name match)", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("include path depth pattern matches only relPath", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"*/fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.False(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("globstar include matches any depth", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("directory include filters by relPath (not basename)", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"sub/**"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.False(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
		assert.True(t, got["sub/inner/deep.go"])
	})

	t.Run("exclude takes precedence over include", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/*.go"},
			Exclude:   []string{"**/deep.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
		assert.False(t, got["sub/inner/deep.go"])
	})

	t.Run("exclude by basename applies at any depth", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/*.go"},
			Exclude:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.False(t, got["fields.go"])
		assert.False(t, got["sub/fields.go"])
		assert.True(t, got["sub/inner/deep.go"])
	})

	t.Run("patterns are relative to root+path base", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "sub",
			Recursive: true,
			Include:   []string{"sub/**"},
		}, out)
		require.NoError(t, err)
		assert.Equal(t, 0, out.Stats.Scanned)
		assert.Len(t, out.Files, 0)
	})

	t.Run("invalid glob pattern does not error but matches nothing", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"["},
		}, out)
		require.NoError(t, err)
		assert.Equal(t, 0, out.Stats.Scanned)
		assert.Len(t, out.Files, 0)
	})

	t.Run("include full uri does not match relPath or basename", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"file:///tmp/fields.go"},
		}, out)
		require.NoError(t, err)
		assert.Equal(t, 0, out.Stats.Scanned)
		assert.Len(t, out.Files, 0)
	})

	t.Run("globstar-only include matches all paths", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/inner/deep.go"])
	})
}

func TestGrepFiles_PatternSemantics(t *testing.T) {
	service := New(dummyAugmenter(t))
	ctx := context.Background()

	rootURI := tempDirURL(t)
	rootFS := strings.TrimPrefix(rootURI, "file://")
	createGrepFixture(t, rootFS)

	t.Run("pattern must not be empty", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern: "   ",
			RootURI: rootURI,
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pattern must not be empty")
	})

	t.Run("invalid regex in pattern errors", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern: "[",
			RootURI: rootURI,
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})

	t.Run("OR semantics via | splits patterns (not regex alternation)", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "AuthMode|Token",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"a.txt", "mixed.txt", "c.log", "**/*.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["a.txt"])
		assert.True(t, got["c.log"])
		assert.True(t, got["mixed.txt"])
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
		// deep.go contains AUTHMODE upper and should not match without caseInsensitive.
		assert.False(t, got["sub/inner/deep.go"])
	})

	t.Run("OR semantics via textual 'or'", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "AuthMode or Token",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"a.txt", "mixed.txt", "c.log", "**/*.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["a.txt"])
		assert.True(t, got["c.log"])
		assert.True(t, got["mixed.txt"])
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
		assert.False(t, got["sub/inner/deep.go"])
	})

	t.Run("excludePattern filters out matching lines", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:        "AuthMode",
			ExcludePattern: "again",
			RootURI:        rootURI,
			Path:           "",
			Recursive:      true,
			Include:        []string{"mixed.txt", "c.log"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["mixed.txt"]) // still has "AuthMode keep"
		assert.False(t, got["c.log"])    // only has "AuthMode again"
	})

	t.Run("caseInsensitive matches uppercase content", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:         "authmode",
			RootURI:         rootURI,
			Path:            "",
			Recursive:       true,
			CaseInsensitive: true,
			Include:         []string{"**/*.go"},
		}, out)
		require.NoError(t, err)
		assert.True(t, pathsSet(out.Files)["sub/inner/deep.go"])
	})

	t.Run("invalid regex in excludePattern errors", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:        "AuthMode",
			ExcludePattern: "[",
			RootURI:        rootURI,
		}, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})
}

func TestGrepFiles_OutputAndLimits(t *testing.T) {
	service := New(dummyAugmenter(t))
	ctx := context.Background()

	rootURI := tempDirURL(t)
	rootFS := strings.TrimPrefix(rootURI, "file://")
	createGrepFixture(t, rootFS)

	t.Run("mode head returns a single snippet from file top", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "fields.go",
			Mode:      "head",
			Lines:     3,
			Bytes:     1024,
			Recursive: false,
		}, out)
		require.NoError(t, err)
		require.Len(t, out.Files, 1)
		require.Len(t, out.Files[0].Snippets, 1)
		assert.Equal(t, 1, out.Files[0].Snippets[0].StartLine)
		assert.Equal(t, 3, out.Files[0].Snippets[0].EndLine)
	})

	t.Run("mode match marks Cut when Bytes truncates snippet", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern: "import",
			RootURI: rootURI,
			Path:    "longline.go",
			Mode:    "match",
			Lines:   2,
			Bytes:   20,
		}, out)
		require.NoError(t, err)
		require.Len(t, out.Files, 1)
		require.GreaterOrEqual(t, len(out.Files[0].Snippets), 1)
		assert.True(t, out.Files[0].Snippets[0].Cut)
	})

	t.Run("maxFiles truncates result set", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/*.go"},
			MaxFiles:  1,
		}, out)
		require.NoError(t, err)
		assert.True(t, out.Stats.Truncated)
		assert.Equal(t, 1, out.Stats.Matched)
		assert.Len(t, out.Files, 1)
	})

	t.Run("maxBlocks truncates snippets in match mode", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "blocks.txt",
			Recursive: false,
			Mode:      "match",
			MaxBlocks: 2,
		}, out)
		require.NoError(t, err)
		require.Len(t, out.Files, 1)
		assert.True(t, out.Stats.Truncated)
		assert.Len(t, out.Files[0].Snippets, 2)
	})

	t.Run("maxSize clips file content before matching", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"fields.go", "big.txt"},
			MaxSize:   1024,
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.False(t, got["big.txt"])
	})

	t.Run("binary files are skipped", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"binary.bin"},
		}, out)
		require.NoError(t, err)
		assert.Equal(t, 1, out.Stats.Scanned)
		assert.Equal(t, 0, out.Stats.Matched)
		assert.Len(t, out.Files, 0)
	})
}

func TestGrepFiles_RecursiveAndFilters(t *testing.T) {
	service := New(dummyAugmenter(t))
	ctx := context.Background()

	rootURI := tempDirURL(t)
	rootFS := strings.TrimPrefix(rootURI, "file://")
	createGrepFixture(t, rootFS)

	t.Run("recursive=false ignores nested matches even if include would match", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: false,
			Include:   []string{"**/fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.False(t, got["sub/fields.go"])
	})

	t.Run("recursive=true plus basename include returns all depths", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("recursive=true with 1-level include matches only that depth", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"*/fields.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.False(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
	})

	t.Run("path=sub and recursive=false only considers direct children of sub", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "sub",
			Recursive: false,
			Include:   []string{"**/*.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.False(t, got["inner/deep.go"])
	})

	t.Run("path=sub makes include patterns relative to sub base", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "sub",
			Recursive: true,
			Include:   []string{"sub/**"},
		}, out)
		require.NoError(t, err)
		assert.Equal(t, 0, out.Stats.Scanned)
		assert.Len(t, out.Files, 0)
	})

	t.Run("exclude still applies when include is broad", func(t *testing.T) {
		out := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "",
			Recursive: true,
			Include:   []string{"**/*.go"},
			Exclude:   []string{"deep.go"},
		}, out)
		require.NoError(t, err)
		got := pathsSet(out.Files)
		assert.True(t, got["fields.go"])
		assert.True(t, got["sub/fields.go"])
		assert.False(t, got["sub/inner/deep.go"])
	})

	t.Run("single-file path ignores recursive", func(t *testing.T) {
		out1 := &GrepOutput{}
		err := service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "sub/inner/deep.go",
			Recursive: false,
		}, out1)
		require.NoError(t, err)
		require.Len(t, out1.Files, 1)

		out2 := &GrepOutput{}
		err = service.grepFiles(ctx, &GrepInput{
			Pattern:   "import",
			RootURI:   rootURI,
			Path:      "sub/inner/deep.go",
			Recursive: true,
		}, out2)
		require.NoError(t, err)
		require.Len(t, out2.Files, 1)

		assert.Equal(t, out1.Files[0].Path, out2.Files[0].Path)
		assert.Equal(t, out1.Files[0].Matches, out2.Files[0].Matches)
	})
}

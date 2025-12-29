package resources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
	agmodel "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/memory"
	convmem "github.com/viant/agently/internal/service/conversation/memory"
)

func tempDirURL(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return "file://" + dir
}

func writeFile(t *testing.T, rootPath string, rel string, content string) string {
	t.Helper()
	// rootPath is a file:// URL; strip prefix to build fs path
	fsRoot := strings.TrimPrefix(rootPath, "file://")
	full := filepath.Join(fsRoot, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return full
}

type testAgentFinder struct {
	agent *agmodel.Agent
}

func (f *testAgentFinder) Find(ctx context.Context, id string) (*agmodel.Agent, error) {
	if f == nil || f.agent == nil {
		return nil, errors.New("agent not configured")
	}
	if f.agent.ID != "" && f.agent.ID != id {
		return nil, errors.New("agent not found")
	}
	return f.agent, nil
}

// TestList_ChildrenOnly (when no recursion) ensures list returns only direct children of the root
// (no self entry) and that item paths are relative to the root.
func TestList_ChildrenOnly(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "a.txt", "alpha")
	writeFile(t, rootURL, "b.md", "bravo")
	_ = os.MkdirAll(strings.TrimPrefix(rootURL, "file://")+"/sub", 0o755)

	svc := New(nil)
	var out ListOutput
	err := svc.list(ctx, &ListInput{RootURI: rootURL}, &out)
	assert.NoError(t, err)
	// Expect only immediate children (no self); include subdirectories as well
	names := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		names = append(names, it.Name)
		// Path should be relative under root
		assert.NotContains(t, it.Path, strings.TrimPrefix(rootURL, "file://"))
	}
	assert.Contains(t, names, "a.txt")
	assert.Contains(t, names, "b.md")
	assert.Contains(t, names, "sub") // dir expected when for no recursive list
	assert.Equal(t, 3, len(out.Items))
}

// TestList_PathIsFile_ReturnsErrorOrSingleItem documents behavior when Path
// points to a file: either an error is returned or a single item is listed.
func TestList_PathIsFile_ReturnsNoItem(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "only.txt", "data")

	svc := New(nil)
	var out ListOutput
	// Listing a file path is commonly an error for directory listing APIs
	err := svc.list(ctx, &ListInput{RootURI: rootURL, Path: "only.txt"}, &out)
	if err == nil {
		assert.Equal(t, 0, len(out.Items))
	}
}

// TestList_RecursiveVsNonRecursive verifies non-recursive listing excludes
// nested files while recursive listing includes them.
func TestList_RecursiveVsNonRecursive(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "top.txt", "t")
	writeFile(t, rootURL, "sub/inner.txt", "i")

	svc := New(nil)

	var flat ListOutput
	err := svc.list(ctx, &ListInput{RootURI: rootURL, Recursive: false}, &flat)
	assert.NoError(t, err)
	gotNames := func(items []ListItem) []string {
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.Path)
		}
		return out
	}
	flatNames := gotNames(flat.Items)

	assert.Contains(t, flatNames, "top.txt")
	assert.Contains(t, flatNames, "sub") // dir expected when Recursive: false in use
	assert.NotContains(t, flatNames, filepath.ToSlash("sub/inner.txt"))
	assert.Equal(t, flat.Total, 2)

	var rec ListOutput
	err = svc.list(ctx, &ListInput{RootURI: rootURL, Recursive: true}, &rec)
	assert.NoError(t, err)
	recNames := gotNames(rec.Items)
	assert.Contains(t, recNames, "top.txt")
	assert.Contains(t, recNames, filepath.ToSlash("sub/inner.txt"))
	assert.NotContains(t, recNames, "sub") // dir not expected when Recursive: true in use
	assert.Equal(t, 2, rec.Total)
}

// TestList_IncludeExclude_Patterns validates include/exclude glob filtering and
// that exclude patterns take precedence over includes.
func TestList_IncludeExclude_Patterns(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "a.txt", "1")
	writeFile(t, rootURL, "b.md", "2")
	writeFile(t, rootURL, "notes/a.md", "3")

	svc := New(nil)
	// include *.md but exclude notes/* (exclude wins)
	var out ListOutput
	err := svc.list(ctx, &ListInput{RootURI: rootURL, Include: []string{"*.md"}, Exclude: []string{"notes/*"}, Recursive: true}, &out)
	assert.NoError(t, err)
	names := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		names = append(names, it.Path)
	}
	assert.Contains(t, names, "b.md")
	assert.NotContains(t, names, filepath.ToSlash("notes/a.md"))
	assert.NotContains(t, names, "a.txt")
}

// TestList_MaxItemsCap confirms that the MaxItems limit caps the number of
// returned items.
func TestList_MaxItemsCap(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	for i := 0; i < 5; i++ {
		writeFile(t, rootURL, fmt.Sprintf("f%d.txt", i), "x")
	}
	svc := New(nil)
	var out ListOutput
	err := svc.list(ctx, &ListInput{RootURI: rootURL, MaxItems: 2}, &out)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(out.Items))
}

// TestList_PathSubfolder checks listing a subfolder via Path returns only items
// under that subtree and excludes siblings at the root level.
func TestList_PathSubfolder(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "sub/a.txt", "1")
	writeFile(t, rootURL, "sub/b.txt", "2")
	writeFile(t, rootURL, "top.txt", "3")

	svc := New(nil)
	var out ListOutput
	err := svc.list(ctx, &ListInput{RootURI: rootURL, Path: "sub"}, &out)
	assert.NoError(t, err)
	names := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		names = append(names, it.Path)
	}
	assert.Contains(t, names, filepath.ToSlash("sub/a.txt"))
	assert.Contains(t, names, filepath.ToSlash("sub/b.txt"))
	assert.NotContains(t, names, "top.txt")
	assert.Equal(t, out.Total, 2)
}

// TestList_RootID_Propagated ensures the resolved root id is propagated into
// ListItem.RootID so callers can attribute results to their origin.
func TestList_RootID_Propagated(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "a.txt", "1")
	svc := New(nil)
	// Pass rootId as the URI (supported fallback in resolveRootID)
	var out ListOutput
	err := svc.list(ctx, &ListInput{RootID: rootURL}, &out)
	assert.NoError(t, err)
	if assert.Greater(t, len(out.Items), 0) {
		assert.NotEmpty(t, out.Items[0].RootID)
	}
}

func TestList_RootID_Include_PatternDepth(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "foo.txt", "root")
	writeFile(t, rootURL, "one/foo.txt", "one")
	writeFile(t, rootURL, "one/two/foo.txt", "two")

	svc := New(nil)

	t.Run("include */foo.txt matches one-level only", func(t *testing.T) {
		var out ListOutput
		err := svc.list(ctx, &ListInput{RootID: rootURL, Recursive: true, Include: []string{"*/foo.txt"}}, &out)
		assert.NoError(t, err)

		paths := make([]string, 0, len(out.Items))
		for _, it := range out.Items {
			paths = append(paths, it.Path)
			assert.Equal(t, rootURL, it.RootID)
		}
		assert.Equal(t, 1, out.Total)
		assert.Contains(t, paths, filepath.ToSlash("one/foo.txt"))
		assert.NotContains(t, paths, "foo.txt")
		assert.NotContains(t, paths, filepath.ToSlash("one/two/foo.txt"))
	})

	t.Run("include **/foo.txt matches any depth", func(t *testing.T) {
		var out ListOutput
		err := svc.list(ctx, &ListInput{RootID: rootURL, Recursive: true, Include: []string{"**/foo.txt"}}, &out)
		assert.NoError(t, err)

		paths := make([]string, 0, len(out.Items))
		for _, it := range out.Items {
			paths = append(paths, it.Path)
			assert.Equal(t, rootURL, it.RootID)
		}
		assert.Equal(t, 3, out.Total)
		assert.Contains(t, paths, "foo.txt")
		assert.Contains(t, paths, filepath.ToSlash("one/foo.txt"))
		assert.Contains(t, paths, filepath.ToSlash("one/two/foo.txt"))
	})
}

func TestList_RootID_Exclude_PatternDepth(t *testing.T) {
	ctx := context.Background()
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "foo.txt", "root")
	writeFile(t, rootURL, "one/foo.txt", "one")
	writeFile(t, rootURL, "one/two/foo.txt", "two")

	svc := New(nil)

	t.Run("exclude */foo.txt removes one-level only", func(t *testing.T) {
		var out ListOutput
		err := svc.list(ctx, &ListInput{RootID: rootURL, Recursive: true, Exclude: []string{"*/foo.txt"}}, &out)
		assert.NoError(t, err)

		paths := make([]string, 0, len(out.Items))
		for _, it := range out.Items {
			paths = append(paths, it.Path)
			assert.Equal(t, rootURL, it.RootID)
		}
		assert.Contains(t, paths, "foo.txt")
		assert.NotContains(t, paths, filepath.ToSlash("one/foo.txt"))
		assert.Contains(t, paths, filepath.ToSlash("one/two/foo.txt"))
	})

	t.Run("exclude **/foo.txt removes any depth", func(t *testing.T) {
		var out ListOutput
		err := svc.list(ctx, &ListInput{RootID: rootURL, Recursive: true, Exclude: []string{"**/foo.txt"}}, &out)
		assert.NoError(t, err)

		paths := make([]string, 0, len(out.Items))
		for _, it := range out.Items {
			paths = append(paths, it.Path)
			assert.Equal(t, rootURL, it.RootID)
		}
		assert.Empty(t, paths)
		assert.Equal(t, 0, out.Total)
	})
}

func TestList_RootID_Local_Include_GlobStar(t *testing.T) {
	rootURL := tempDirURL(t)
	writeFile(t, rootURL, "foo.txt", "root")
	writeFile(t, rootURL, "one/foo.txt", "one")
	writeFile(t, rootURL, "one/two/foo.txt", "two")

	agentID := "test-agent"
	convClient := convmem.New()
	conv := apiconv.NewConversation()
	conv.SetId("conv-1")
	conv.SetAgentId(agentID)
	assert.NoError(t, convClient.PatchConversations(context.Background(), conv))

	svc := New(nil,
		WithConversationClient(convClient),
		WithAgentFinder(&testAgentFinder{agent: &agmodel.Agent{
			Identity: agmodel.Identity{ID: agentID},
			Resources: []*agmodel.Resource{
				{ID: "local", URI: rootURL, Role: "user"},
			},
		}}),
	)

	ctx := memory.WithConversationID(context.Background(), "conv-1")
	var out ListOutput
	err := svc.list(ctx, &ListInput{
		RootID:    "local",
		Recursive: true,
		Include:   []string{"**/foo.txt"},
		MaxItems:  50,
		Path:      "",
		Exclude:   nil,
		RootURI:   "",
	}, &out)
	assert.NoError(t, err)

	paths := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		paths = append(paths, it.Path)
		assert.Equal(t, "local", it.RootID)
	}
	assert.Equal(t, 3, out.Total)
	assert.Contains(t, paths, "foo.txt")
	assert.Contains(t, paths, filepath.ToSlash("one/foo.txt"))
	assert.Contains(t, paths, filepath.ToSlash("one/two/foo.txt"))
}

package patch

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
)

func TestSession_ApplyPatch_UpdateFile_WhenFileEmpty(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	session, err := NewSession()
	assert.NoError(t, err)

	// Create an empty file
	path := "mem://localhost/empty.txt"
	err = fs.Upload(ctx, path, 0o777, strings.NewReader(""))
	assert.NoError(t, err)

	patchText := `*** Begin Patch
*** Update File: mem://localhost/empty.txt
@@
+hello
*** End Patch`

	err = session.ApplyPatch(ctx, patchText)
	assert.NoError(t, err)

	err = session.Commit(ctx)
	assert.NoError(t, err)

	data, err := fs.DownloadWithURL(ctx, path)
	assert.NoError(t, err)
	assert.Equal(t, "hello\n", string(data))
}

func TestSession_ApplyPatch_UpdateFile_UsesChangeContext(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	session, err := NewSession()
	assert.NoError(t, err)

	path := "mem://localhost/repeat.txt"
	err = fs.Upload(ctx, path, 0o777, strings.NewReader("beta\nbeta\nbeta\n"))
	assert.NoError(t, err)

	patchText := `*** Begin Patch
*** Update File: mem://localhost/repeat.txt
@@ beta
-beta
+gamma
*** End Patch`

	err = session.ApplyPatch(ctx, patchText)
	assert.NoError(t, err)

	err = session.Commit(ctx)
	assert.NoError(t, err)

	data, err := fs.DownloadWithURL(ctx, path)
	assert.NoError(t, err)
	assert.Equal(t, "beta\ngamma\nbeta\n", string(data))
}

func TestSession_ApplyPatch_UpdateFile_RepeatedLargeBlock(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	session, err := NewSession()
	assert.NoError(t, err)

	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("alpha\nbeta\ngamma\n")
	}
	b.WriteString("alpha\nbeta\ndelta\n")
	for i := 0; i < 200; i++ {
		b.WriteString("alpha\nbeta\ngamma\n")
	}

	path := "mem://localhost/large.txt"
	err = fs.Upload(ctx, path, 0o777, strings.NewReader(b.String()))
	assert.NoError(t, err)

	patchText := `*** Begin Patch
*** Update File: mem://localhost/large.txt
@@ alpha
 alpha
-beta
-delta
+beta
+epsilon
*** End Patch`

	err = session.ApplyPatch(ctx, patchText)
	assert.NoError(t, err)

	err = session.Commit(ctx)
	assert.NoError(t, err)

	data, err := fs.DownloadWithURL(ctx, path)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "alpha\nbeta\nepsilon\n")
	assert.NotContains(t, string(data), "alpha\nbeta\ndelta\n")
}

func TestSession_ApplyPatch_UpdateFile_EOFMarkerAppends(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	session, err := NewSession()
	assert.NoError(t, err)

	path := "mem://localhost/eof.txt"
	err = fs.Upload(ctx, path, 0o777, strings.NewReader("one\ntwo\n"))
	assert.NoError(t, err)

	patchText := `*** Begin Patch
*** Update File: mem://localhost/eof.txt
@@
 one
 two
+three
*** End of File
*** End Patch`

	err = session.ApplyPatch(ctx, patchText)
	assert.NoError(t, err)

	err = session.Commit(ctx)
	assert.NoError(t, err)

	data, err := fs.DownloadWithURL(ctx, path)
	assert.NoError(t, err)
	assert.Equal(t, "one\ntwo\nthree\n", string(data))
}

func TestSession_ApplyPatch_UpdateFile_EOFReplaceMissingTrailingNewline(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	session, err := NewSession()
	assert.NoError(t, err)

	path := "mem://localhost/noeof.txt"
	err = fs.Upload(ctx, path, 0o777, strings.NewReader("foo\nbar"))
	assert.NoError(t, err)

	patchText := `*** Begin Patch
*** Update File: mem://localhost/noeof.txt
@@
-bar
+baz
*** End of File
*** End Patch`

	err = session.ApplyPatch(ctx, patchText)
	assert.NoError(t, err)

	err = session.Commit(ctx)
	assert.NoError(t, err)

	data, err := fs.DownloadWithURL(ctx, path)
	assert.NoError(t, err)
	assert.Equal(t, "foo\nbaz\n", string(data))
}

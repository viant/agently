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

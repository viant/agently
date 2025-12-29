package resources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_SignaturesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.go")
	content := strings.Join([]string{
		"package foo",
		"",
		"import \"fmt\"",
		"",
		"func Add(a, b int) int {",
		"    return a + b",
		"}",
		"",
		"func main() {",
		"    fmt.Println(Add(1, 2))",
		"}",
		"",
		"var unused = 1",
		"",
	}, "\n")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	svc := New(nil)
	uri := "file://" + path

	t.Run("extracts signatures", func(t *testing.T) {
		out := &ReadOutput{}
		err := svc.read(context.Background(), &ReadInput{URI: uri, Mode: "signatures"}, out)
		require.NoError(t, err)

		assert.Equal(t, "signatures", out.ModeApplied)
		assert.Equal(t, "package foo\nimport \"fmt\"\nfunc Add(a, b int) int {\nfunc main() {", out.Content)
		assert.NotContains(t, out.Content, "return a + b")
		assert.NotContains(t, out.Content, "fmt.Println")
	})

	t.Run("caps by maxBytes", func(t *testing.T) {
		out := &ReadOutput{}
		err := svc.read(context.Background(), &ReadInput{URI: uri, Mode: "signatures", MaxBytes: 10}, out)
		require.NoError(t, err)

		assert.Equal(t, "signatures", out.ModeApplied)
		assert.Equal(t, "package fo", out.Content)
	})
}

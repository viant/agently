package agently

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestManageWorkspaceDeepLink(t *testing.T) {
	const html = "<html>workspace shell</html>"
	bundle := servedUIBundle{FS: fstest.MapFS{}, Index: []byte(html)}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	for _, dist := range []string{"", dir} {
		handler := newRouter(http.NotFoundHandler(), http.NotFoundHandler(), http.NotFoundHandler(), dist, bundle)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/ui/workspace?tab=starting-tasks", nil))
		if w.Code != 200 || w.Body.String() != html {
			t.Fatalf("workspace deep link returned HTTP %d and %q", w.Code, w.Body.String())
		}
	}
}

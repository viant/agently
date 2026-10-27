package agently

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	deployui "github.com/viant/agently/deployment/ui"
)

func TestRouterServesCompleteEmbeddedBundle(t *testing.T) {
	unused := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("UI request reached API: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
	handler := newRouter(unused, unused, unused, "", servedUIBundle{Name: "test", FS: deployui.FS, Index: deployui.Index})
	err := fs.WalkDir(deployui.FS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		expected, err := deployui.FS.ReadFile(name)
		if err != nil {
			return err
		}
		// net/http redirects index.html to the root URL.
		url := "/" + name
		if name == "index.html" {
			url = "/"
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s returned %d", url, response.Code)
		}
		if !bytes.Equal(expected, response.Body.Bytes()) {
			t.Errorf("%s did not serve its embedded content", url)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

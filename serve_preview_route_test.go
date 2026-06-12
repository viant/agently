package agently

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewRouter_ServesLookupChipPreviewAsHTML(t *testing.T) {
	uiDir := t.TempDir()
	indexPath := filepath.Join(uiDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html><body>preview-shell</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	apiCalled := false
	metaCalled := false
	speechCalled := false

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		http.Error(w, "api", http.StatusTeapot)
	})
	meta := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metaCalled = true
		http.Error(w, "meta", http.StatusTeapot)
	})
	speech := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		speechCalled = true
		http.Error(w, "speech", http.StatusTeapot)
	})

	bundle := servedUIBundle{
		Name: "test",
		FS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html><body>embedded</body></html>")},
		},
		Index: []byte("<html><body>embedded</body></html>"),
	}

	handler := newRouter(api, meta, speech, uiDir, bundle)
	for _, path := range []string{"/lookup-chip-preview", "/ui/lookup-chip-preview"} {
		apiCalled = false
		metaCalled = false
		speechCalled = false

		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if apiCalled || metaCalled || speechCalled {
			t.Fatalf("expected preview route %s to serve UI shell directly, api=%v meta=%v speech=%v", path, apiCalled, metaCalled, speechCalled)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 for %s, got %d", path, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "preview-shell") {
			t.Fatalf("expected local UI index for %s, got %q", path, body)
		}
		if got := w.Header().Get("Cache-Control"); got != htmlCacheControl {
			t.Fatalf("want Cache-Control %q for %s, got %q", htmlCacheControl, path, got)
		}
	}

	// Keep the compiler honest about the bundle type.
	if _, ok := bundle.FS.(fs.FS); !ok {
		t.Fatalf("bundle FS must satisfy fs.FS")
	}
}

func TestNewRouter_ForwardsUploadToAPI(t *testing.T) {
	apiCalled := false
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		if r.URL.Path != "/upload" {
			t.Fatalf("unexpected API path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	meta := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("meta should not handle %s", r.URL.Path)
	})
	speech := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("speech should not handle %s", r.URL.Path)
	})
	bundle := servedUIBundle{
		Name:  "test",
		FS:    fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}},
		Index: []byte("<html></html>"),
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	w := httptest.NewRecorder()
	newRouter(api, meta, speech, "", bundle).ServeHTTP(w, req)

	if !apiCalled {
		t.Fatalf("expected /upload to reach API handler")
	}
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", w.Code)
	}
}

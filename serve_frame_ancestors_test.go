package agently

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestNewRouter_EmitsFrameAncestorsPolicy verifies the host-page framing
// policy is stamped onto every response path served by the Agently router,
// including the top-level conversation HTML shell, asset responses, and
// requests that fall through to the embedded UI server.
//
// This is the host-page mitigation for the "host-app XSS" risk documented
// under the MCP UI plan ("Risk: host-app XSS"). The guest srcdoc CSP is a
// separate concern delivered inside guest HTML via meta tags; it cannot
// substitute for a real HTTP-level frame-ancestors directive on the host.
func TestNewRouter_EmitsFrameAncestorsPolicy(t *testing.T) {
	uiDir := t.TempDir()
	indexPath := filepath.Join(uiDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html><body>shell</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assetPath := filepath.Join(uiDir, "assets")
	if err := os.MkdirAll(assetPath, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetPath, "app.js"), []byte("/*js*/"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	meta := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	speech := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	bundle := servedUIBundle{
		Name: "test",
		FS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html><body>embedded</body></html>")},
		},
		Index: []byte("<html><body>embedded</body></html>"),
	}

	handler := newRouter(api, meta, speech, uiDir, bundle)

	cases := []struct {
		name string
		path string
	}{
		{"root", "/"},
		{"ui_shell", "/ui"},
		{"conversation", "/conversation/abc-123"},
		{"ui_conversation", "/ui/conversation/abc-123"},
		{"v1_conversation", "/v1/conversation/abc-123"},
		{"lookup_chip_preview", "/lookup-chip-preview"},
		{"forge_window", "/mcp-ui/forge-window"},
		{"oauth_callback", "/v1/api/auth/oauth/callback"},
		{"asset", "/assets/app.js"},
		{"api", "/v1/healthz"},
		{"healthz", "/healthz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if got, want := w.Header().Get("Content-Security-Policy"), frameAncestorsPolicy; got != want {
				t.Fatalf("path %s: Content-Security-Policy = %q, want %q", tc.path, got, want)
			}
			if got, want := w.Header().Get("X-Frame-Options"), frameOptionsPolicy; got != want {
				t.Fatalf("path %s: X-Frame-Options = %q, want %q", tc.path, got, want)
			}
		})
	}
}

// TestFrameAncestorsPolicy_Literal pins the exact directive emitted on the
// wire so future refactors of the wrapper do not silently weaken the
// host-page mitigation (e.g. flipping 'self' to '*' or dropping the
// directive name).
func TestFrameAncestorsPolicy_Literal(t *testing.T) {
	if frameAncestorsPolicy != "frame-ancestors 'self'" {
		t.Fatalf("frameAncestorsPolicy = %q, want \"frame-ancestors 'self'\"", frameAncestorsPolicy)
	}
	if frameOptionsPolicy != "SAMEORIGIN" {
		t.Fatalf("frameOptionsPolicy = %q, want SAMEORIGIN", frameOptionsPolicy)
	}
}

func TestNewRouter_LetsRealOAuthCallbackReachAPI(t *testing.T) {
	uiDir := t.TempDir()
	indexPath := filepath.Join(uiDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html><body>shell</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	apiCalled := false
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	meta := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	speech := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	bundle := servedUIBundle{
		Name: "test",
		FS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html><body>embedded</body></html>")},
		},
		Index: []byte("<html><body>embedded</body></html>"),
	}

	handler := newRouter(api, meta, speech, uiDir, bundle)
	req := httptest.NewRequest(http.MethodGet, "/v1/api/auth/oauth/callback?code=abc&state=xyz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !apiCalled {
		t.Fatalf("expected real oauth callback with code/state to reach api handler")
	}
	if got := w.Body.String(); got != `{"status":"ok"}` {
		t.Fatalf("unexpected api response body %q", got)
	}
}

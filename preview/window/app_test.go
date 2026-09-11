package windowpreview

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func example(t *testing.T) *App {
	t.Helper()
	a, e := New(Config{Root: "examples/projects"})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func TestCatalogAndNativeLinks(t *testing.T) {
	a := example(t)
	w, e := a.LoadWindow(context.Background(), "projects")
	if e != nil {
		t.Fatal(e)
	}
	if w.DataSource["projects"].DataSourceRef != "" {
		t.Fatal("shared reference must not become a self-referencing runtime source")
	}
	if w.DataSource["projects"].Service.URI != "/v1/api/datasources/projects/fetch" {
		t.Fatal("wrong fetch service")
	}
	link := w.View.Content.Table.Columns[1].Link
	if link == nil || link.Kind != "window" || link.WindowKey != "project-tasks" {
		t.Fatal("native cross-window link lost")
	}
	if _, e = a.LoadWindow(context.Background(), "../outside"); e == nil {
		t.Fatal("window allowlist bypass")
	}
}
func TestWindowFetchUsesMCP(t *testing.T) {
	a := example(t)
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	for _, tc := range []struct {
		id, body string
		count    int
		first    any
		status   int
	}{
		{"projects", `{"inputs":{"size":5,"offset":5}}`, 3, float64(6), 200},
		{"tasks", `{"inputs":{"projectId":2}}`, 3, float64(2), 200},
		{"projects", `{"inputs":{"query":{"filter":{"field":"region","op":"eq","value":"North"},"projection":{"fields":["projectId","budget"]},"orderBy":[{"field":"budget","direction":"desc"}],"page":{"limit":1,"offset":0}}}}`, 1, float64(7), 200},
		{"projects", `{"inputs":{},"variant":"empty"}`, 0, nil, 200},
		{"projects", `{"inputs":{},"variant":"error"}`, 0, nil, 422},
		{"projects", `{"inputs":{"size":0}}`, 0, nil, 422},
	} {
		t.Run(tc.body, func(t *testing.T) {
			r, e := server.Client().Post(server.URL+"/v1/api/datasources/"+tc.id+"/fetch", "application/json", strings.NewReader(tc.body))
			if e != nil {
				t.Fatal(e)
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			if r.StatusCode != tc.status {
				t.Fatalf("status %d: %s", r.StatusCode, b)
			}
			if tc.status != 200 {
				return
			}
			var out struct {
				Rows []map[string]any `json:"rows"`
			}
			if e = json.Unmarshal(b, &out); e != nil {
				t.Fatal(e)
			}
			if len(out.Rows) != tc.count {
				t.Fatalf("rows: %s", b)
			}
			if tc.count > 0 && out.Rows[0]["projectId"] != tc.first {
				t.Fatal(string(b))
			}
		})
	}
}
func TestImportBoundary(t *testing.T) {
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("examples/projects")); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(root, "windows", "projects.yaml")
	_ = os.WriteFile(p, []byte("$import: https://example.com/window.yaml\n"), 0644)
	if _, e := New(Config{Root: root}); e == nil {
		t.Fatal("remote import accepted")
	}
}
func TestMCPFailureNoFixtureFallback(t *testing.T) {
	a := example(t)
	ep := a.workspace.Endpoints["mockData"]
	ep.BaseURL = "http://127.0.0.1:1/mcp"
	a.workspace.Endpoints["mockData"] = ep
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	r, e := server.Client().Post(server.URL+"/v1/api/datasources/projects/fetch", "application/json", strings.NewReader(`{"inputs":{}}`))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	if r.StatusCode != 422 {
		t.Fatal("MCP failure fell back to fixtures")
	}
}

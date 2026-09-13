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

	"github.com/viant/forge/backend/types"
)

func TestReadOnlyPreservesPresentationDataHooks(t *testing.T) {
	a := example(t)
	a.config.ReadOnly = true
	definition := a.workspace.Windows["projects"]
	definition.AuthorizationSnapshot = map[string]interface{}{"principal": map[string]interface{}{"roles": []string{"PROJECT_REVIEWER"}}}
	a.workspace.Windows["projects"] = definition
	source := a.workspace.DataSources["projects"]
	source.On = []*types.Execute{{Event: "onSuccess", Handler: "Projects.resolveLabels"}}
	a.workspace.DataSources["projects"] = source
	w, err := a.LoadWindow(context.Background(), "projects")
	if err != nil {
		t.Fatal(err)
	}
	ds := w.DataSource["projects"]
	if len(ds.On) != 1 || ds.On[0].Handler != "Projects.resolveLabels" {
		t.Fatal("read-only preview dropped label-resolution lifecycle hook")
	}
	if ds.Service.Endpoint != "preview" || ds.Service.URI != "/v1/api/datasources/projects/fetch" {
		t.Fatal("presentation hook datasource escaped the synthetic preview bridge")
	}
	principal, ok := w.AuthorizationSnapshot["principal"].(map[string]interface{})
	if !ok || principal["roles"].([]string)[0] != "PROJECT_REVIEWER" {
		t.Fatal("read-only preview did not use its window's configured mock principal")
	}
}

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

func TestWindowEndpointHonorsPhoneTarget(t *testing.T) {
	a := example(t)
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/windows/projects?platform=android&formFactor=phone&surface=app")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status %d: %s", response.StatusCode, body)
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

func TestWorkspaceStylesAreVersionedAndReloaded(t *testing.T) {
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("examples/projects")); e != nil {
		t.Fatal(e)
	}
	styles := filepath.Join(root, "extension", "forge", "styles")
	if e := os.MkdirAll(styles, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(styles, "manifest.yaml"), []byte("version: 1\nfiles: [preview.css]\n"), 0600); e != nil {
		t.Fatal(e)
	}
	writeCSS := func(value string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(styles, "preview.css"), []byte(".agently-workspace { --preview-accent: "+value+"; }\n"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	writeCSS("#123456")
	a, e := New(Config{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	load := func() (string, string) {
		t.Helper()
		response, err := server.Client().Get(server.URL + "/api/workspace")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var metadata struct {
			UIStyles *struct{ Revision, Href string } `json:"uiStyles"`
		}
		if err = json.NewDecoder(response.Body).Decode(&metadata); err != nil || metadata.UIStyles == nil {
			t.Fatalf("workspace styles missing: %v", err)
		}
		cssResponse, err := server.Client().Get(server.URL + metadata.UIStyles.Href)
		if err != nil {
			t.Fatal(err)
		}
		defer cssResponse.Body.Close()
		body, _ := io.ReadAll(cssResponse.Body)
		if cssResponse.Header.Get("X-Content-Type-Options") != "nosniff" || cssResponse.Header.Get("Cache-Control") != "private, no-cache" {
			t.Fatal("workspace stylesheet security headers missing")
		}
		return metadata.UIStyles.Revision, string(body)
	}
	firstRevision, firstCSS := load()
	if !strings.Contains(firstCSS, "#123456") {
		t.Fatal(firstCSS)
	}
	writeCSS("#654321")
	secondRevision, secondCSS := load()
	if firstRevision == secondRevision || !strings.Contains(secondCSS, "#654321") {
		t.Fatalf("stylesheet did not reload: %s %s", firstRevision, secondRevision)
	}
	response, err := server.Client().Get(server.URL + "/v1/workspace/ui/styles/../../preview.css")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("unsafe stylesheet path returned %d", response.StatusCode)
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

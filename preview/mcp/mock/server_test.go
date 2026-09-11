package mock_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viant/agently/preview/datasource"
	"github.com/viant/agently/preview/mcp/mock"
	"github.com/viant/forge/backend/types"
)

func setup(t *testing.T) (string, *datasource.Client) {
	t.Helper()
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "ds"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "variants", "empty", "ds"), 0755)
	for file, body := range map[string]string{"ds/projects.json": `{"status":"ok","rows":[{"project":"Alpha","owner":"North","units":30},{"project":"Beta","owner":"Central","units":40},{"project":"Alpha","owner":"South","units":10},{"project":"Alpha","owner":"West","units":30}],"hasMore":false,"meta":{"source":"synthetic"}}`, "variants/empty/ds/projects.json": `{"status":"ok","rows":[],"hasMore":false}`} {
		if e := os.WriteFile(filepath.Join(root, file), []byte(body), 0644); e != nil {
			t.Fatal(e)
		}
	}
	server, e := mock.New(mock.Config{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	httpServer := httptest.NewServer(mock.LocalOnly(server.HTTPHandler()))
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	client, e := datasource.Dial(ctx, httpServer.URL+"/mcp")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(client.Close)
	return root, client
}
func TestRealMCPFixtureQuery(t *testing.T) {
	_, client := setup(t)
	ctx := context.Background()
	tools, e := client.ListTools(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "projects" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	body, e := client.Call(ctx, "projects", map[string]any{"query": map[string]any{"filter": map[string]any{"field": "project", "op": "eq", "value": "Alpha"}, "projection": map[string]any{"fields": []any{"owner", map[string]any{"field": "units", "alias": "total"}}}, "orderBy": []any{map[string]any{"field": "total", "direction": "desc"}}, "page": map[string]any{"limit": 1, "offset": 1}}})
	if e != nil {
		t.Fatal(e)
	}
	var result struct {
		Rows    []map[string]any `json:"rows"`
		HasMore bool             `json:"hasMore"`
		Meta    map[string]any   `json:"meta"`
	}
	if e = json.Unmarshal(body, &result); e != nil {
		t.Fatal(e)
	}
	if len(result.Rows) != 1 || result.Rows[0]["owner"] != "West" || result.Rows[0]["total"] != 30.0 || len(result.Rows[0]) != 2 || !result.HasMore || result.Meta["source"] != "synthetic" {
		t.Fatalf("unexpected result: %s", body)
	}
	empty, e := client.Call(ctx, "projects", map[string]any{"variant": "empty"})
	if e != nil || !strings.Contains(string(empty), `"rows":[]`) {
		t.Fatalf("%s: %v", empty, e)
	}
	if _, e = client.Call(ctx, "projects", map[string]any{"variant": "../../outside"}); e == nil {
		t.Fatal("path traversal accepted")
	}
	if _, e = client.Call(ctx, "projects", map[string]any{"query": map[string]any{"page": map[string]any{"limit": 0}}}); e == nil {
		t.Fatal("invalid page accepted")
	}
}
func TestFixtureReloadAndRawEnvelope(t *testing.T) {
	root, client := setup(t)
	path := filepath.Join(root, "ds", "projects.json")
	body := `{"status":"ok","rows":[{"project":"Updated","units":77}],"hasMore":false}`
	_ = os.WriteFile(path, []byte(body), 0644)
	got, e := client.Call(context.Background(), "projects", map[string]any{})
	if e != nil || !strings.Contains(string(got), "Updated") {
		t.Fatalf("%s: %v", got, e)
	}
}
func TestPathEscape(t *testing.T) {
	root := t.TempDir()
	_ = os.Mkdir(filepath.Join(root, "ds"), 0755)
	outside := filepath.Join(t.TempDir(), "secret.json")
	_ = os.WriteFile(outside, []byte(`{}`), 0644)
	_ = os.Symlink(outside, filepath.Join(root, "ds", "escape.json"))
	if _, e := mock.New(mock.Config{Root: root}); e == nil {
		t.Fatal("symlink escaped root")
	}
}
func TestEndpointResolution(t *testing.T) {
	service := &types.Service{Endpoint: "fixtures", URI: "projects", Method: "POST"}
	url, tool, e := datasource.Resolve(service, map[string]datasource.Endpoint{"fixtures": {Type: "mcp", BaseURL: "/mcp", Transport: "streamable"}}, "http://127.0.0.1:8095")
	if e != nil || url != "http://127.0.0.1:8095/mcp" || tool != "projects" {
		t.Fatalf("%s %s %v", url, tool, e)
	}
}
func TestTabularProjection(t *testing.T) {
	body := json.RawMessage(`{"status":"ok","data":[{"name":"projects","columns":[{"name":"project","type":"string"},{"name":"units","type":"integer","format":"compactNumber"}],"rows":[["Alpha",30],["Beta",40]],"hasMore":false}],"meta":{"source":"synthetic"}}`)
	result, e := mock.ApplyQuery(body, mock.Query{Projection: &mock.Projection{Fields: []mock.Field{{Field: "units", Alias: "total"}, {Field: "project"}}}, OrderBy: []mock.Order{{Field: "total", Direction: "desc"}}, Page: &mock.Page{Limit: 1}})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(result)
	if !strings.Contains(string(b), `"rows":[[40,"Beta"]]`) || !strings.Contains(string(b), `"format":"compactNumber"`) {
		t.Fatal(string(b))
	}
}
func TestQueryValidationOverMCP(t *testing.T) {
	_, client := setup(t)
	for _, q := range []map[string]any{
		{"filter": map[string]any{"field": "units", "op": "eq", "value": "30"}},
		{"filter": map[string]any{"and": []any{}}},
		{"projection": map[string]any{"fields": []string{"missing"}}},
		{"orderBy": []any{map[string]any{"field": "units", "direction": "up"}}},
		{"projection": map[string]any{"measures": []any{map[string]any{"field": "units", "aggregation": "sum"}}}},
	} {
		if _, e := client.Call(context.Background(), "projects", map[string]any{"query": q}); e == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}

package reportpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/viant/agently/preview/datasource"
	preview "github.com/viant/agently/preview/report/provider"
)

func TestReportDatasourceUsesRealMCP(t *testing.T) {
	folder := filepath.Join("examples", "demo")
	server, e := MockServer(Config{Folder: folder})
	if e != nil {
		t.Fatal(e)
	}
	handler := server.HTTPHandler()
	var calls atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			var message struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(body, &message)
			if message.Method == "tools/call" {
				calls.Add(1)
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer host.Close()
	p, e := preview.Load(folder, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = p.UseMCP(context.Background(), host.URL); e != nil {
		t.Fatal(e)
	}
	c, e := p.Compile(nil, true)
	if e != nil {
		t.Fatal(e)
	}
	if calls.Load() < 5 || len(c.Results["operationsEvidence"].Rows) != 12 {
		t.Fatalf("report did not fetch full datasets through MCP: calls=%d", calls.Load())
	}
	p.Endpoints["mockReports"] = datasource.Endpoint{Type: "mcp", BaseURL: "http://127.0.0.1:1/mcp"}
	if _, e = p.Execute("operations", preview.Query{}); e == nil {
		t.Fatal("unavailable MCP server silently fell back to fixtures")
	}
}
func TestReportMCPColumnSelection(t *testing.T) {
	folder := filepath.Join("examples", "demo")
	server, e := MockServer(Config{Folder: folder})
	if e != nil {
		t.Fatal(e)
	}
	host := httptest.NewServer(server.HTTPHandler())
	defer host.Close()
	p, e := preview.Load(folder, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = p.UseMCP(context.Background(), host.URL); e != nil {
		t.Fatal(e)
	}
	r, e := p.Execute("projectTargets", preview.Query{Projection: &preview.Projection{Fields: []preview.Field{{Field: "budget", Alias: "amount"}, {Field: "project"}}}, Filter: &preview.Predicate{Field: "project", Op: "eq", Value: "Process Improvement"}, Page: &preview.Page{Limit: 1}})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Rows) != 1 || len(r.Rows[0]) != 2 || r.Rows[0]["amount"] != 48000.0 {
		t.Fatalf("unexpected projection: %+v", r)
	}
}

package reportpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/viant/agently-core/service/reportdefinition"
	"github.com/viant/agently/preview/mcp/mock"
	preview "github.com/viant/agently/preview/report/provider"
)

func activatedTestEnvelope(t *testing.T) []byte {
	t.Helper()
	p, err := preview.Load(filepath.Join("examples", "demo"), "")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := p.Compile(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	value := reportdefinition.ActivatedReport{
		GroupID: "sample", ReportID: "operations", ArtifactID: "artifact-1",
		Version: 3, DraftRevision: 2, ContentDigest: "digest",
		ReportDocument: compiled.ReportDocument, ReportSpec: compiled.ReportSpec,
		ReportFill: compiled.ReportFill, ReportPrint: compiled.ReportPrint,
		ComputedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"status": "ok", "data": value})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestActivateLocalMCP(t *testing.T) {
	body := activatedTestEnvelope(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "ds"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ds", "activate.json"), body, 0644); err != nil {
		t.Fatal(err)
	}
	server, err := mock.New(mock.Config{Root: root, Tools: map[string]mock.Tool{
		"selected_activate": {File: "activate.json"},
	}, Transform: func(_ context.Context, call mock.Call) (any, error) {
		if call.Tool != "selected_activate" || call.Arguments["groupId"] != "sample" || call.Arguments["reportId"] != "operations" || call.Arguments["limit"] != float64(25) {
			t.Errorf("unexpected MCP call: %+v", call)
		}
		if !reflect.DeepEqual(call.Arguments["context"], map[string]any{"entities": []any{map[string]any{"type": "project", "id": "101"}}}) || !reflect.DeepEqual(call.Arguments["parameters"], map[string]any{"region": "West"}) {
			t.Errorf("context/parameters were not forwarded: %+v", call.Arguments)
		}
		var result any
		return result, json.Unmarshal(call.Body, &result)
	}})
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(server.HTTPHandler())
	defer host.Close()
	contextFile := filepath.Join(root, "context.json")
	parametersFile := filepath.Join(root, "parameters.json")
	if err := os.WriteFile(contextFile, []byte(`{"entities":[{"type":"project","id":"101"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parametersFile, []byte(`{"region":"West"}`), 0644); err != nil {
		t.Fatal(err)
	}
	base := []string{"activate", "--mcp-url", host.URL + "/mcp", "--tool", "selected_activate", "--group-id", "sample", "--report-id", "operations", "--context", contextFile, "--parameters", parametersFile, "--limit", "25"}
	var output bytes.Buffer
	if err := RunCLI(context.Background(), base, &output, &output); err != nil {
		t.Fatal(err)
	}
	var activated reportdefinition.ActivatedReport
	if err := json.Unmarshal(output.Bytes(), &activated); err != nil || activated.ArtifactID != "artifact-1" {
		t.Fatalf("JSON output: %s, %v", output.String(), err)
	}
	for _, tc := range []struct{ ext, prefix string }{{".html", "<!doctype html>"}, {".pdf", "%PDF-"}} {
		path := filepath.Join(root, "report"+tc.ext)
		output.Reset()
		if err := RunCLI(context.Background(), append(append([]string{}, base...), "--out", path), &output, &output); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || !bytes.HasPrefix(data, []byte(tc.prefix)) {
			t.Fatalf("%s output: %v, %.80s", tc.ext, err, data)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("report export must be private: %v, %v", info, err)
		}
		if tc.ext == ".html" && (!bytes.Contains(data, []byte("</html>")) || !bytes.Contains(data, []byte("class=\"page\""))) {
			t.Fatal("HTML did not contain complete Forge pages")
		}
	}
}

func TestActivateCallerAndValidation(t *testing.T) {
	body := activatedTestEnvelope(t)
	args := []string{"--mcp-url", "http://localhost:1/mcp", "--group-id", "sample", "--report-id", "operations"}
	var output bytes.Buffer
	called := false
	caller := func(_ context.Context, endpoint, tool string, values map[string]any) (json.RawMessage, error) {
		called = true
		if endpoint != "http://localhost:1/mcp" || tool != defaultActivateTool || values["limit"] != 1000 {
			t.Fatalf("unexpected default call: %s %s %+v", endpoint, tool, values)
		}
		return body, nil
	}
	if err := runActivateCLI(context.Background(), args, &output, &output, caller); err != nil || !called {
		t.Fatalf("default caller: %v, called=%v", err, called)
	}
	output.Reset()
	bad := strings.Replace(string(body), `"groupId":"sample"`, `"groupId":"other"`, 1)
	err := runActivateCLI(context.Background(), args, &output, &output, func(context.Context, string, string, map[string]any) (json.RawMessage, error) {
		return []byte(bad), nil
	})
	if err == nil || output.Len() != 0 {
		t.Fatalf("invalid identity produced output: %v %s", err, output.String())
	}
	if err := runActivateCLI(context.Background(), append(args, "--limit", "0"), &output, &output, caller); err == nil {
		t.Fatal("accepted invalid limit")
	}
	if err := runActivateCLI(context.Background(), append(args, "--limit", "1001"), &output, &output, caller); err == nil {
		t.Fatal("accepted limit above the server cap")
	}
	contextFile := filepath.Join(t.TempDir(), "invalid-context.json")
	if err := os.WriteFile(contextFile, []byte(`{"auth":{"roles":["admin"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runActivateCLI(context.Background(), append(args, "--context", contextFile), &output, &output, caller); err == nil {
		t.Fatal("caller-supplied auth context was accepted")
	}
	if err := os.WriteFile(contextFile, []byte(strings.Repeat(" ", (64<<10)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readActivateObject(contextFile); err == nil {
		t.Fatal("oversized context file was accepted")
	}
}

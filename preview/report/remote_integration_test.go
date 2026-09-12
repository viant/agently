package reportpreview

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viant/agently/preview/mcp/mock"
)

func TestRemoteMCPDefinitionAndDataBootstrap(t *testing.T) {
	catalogRoot, fixtureRoot := t.TempDir(), t.TempDir()
	write := func(root, name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(catalogRoot, "group.yaml", "kind: forge.reporting.group\nid: performance\n")
	write(catalogRoot, "builder.yaml", "reportBuilder:\n  presentationProfileRefs: [./profiles/reports.json]\n")
	write(catalogRoot, "definitions.json", `{"version":"1","views":[{"reportId":"channels","name":"Channels","visualProfile":"channels","fields":[{"name":"channel","role":"dimension"},{"name":"units","role":"measure"}]}]}`)
	write(catalogRoot, "profiles/reports.json", `{"kind":"forge.reporting.presentationProfileCatalog","schemaVersion":1,"views":[{"reportId":"channels","visualProfile":"channels","tabs":[{"id":"overview","title":"Overview","blockIds":["evidence"]}],"blocks":[{"id":"evidence","kind":"tableBlock","title":"Evidence","datasetRef":"primary","columns":[{"key":"channel","label":"Channel"},{"key":"units","label":"Units","format":"compactNumber"}]}]}]}`)
	write(fixtureRoot, "preview.yaml", `version: 1
groupId: performance
reportId: channels
mcp:
  tool: ReportRun
  defaultFile: default.json
  routes:
    - {match: {request.action: run}, file: primary.json}
  query: {dimensions: request.dimensions, measures: request.measures, limit: request.limit, offset: request.offset}
  definition:
    tool: ReportRun
    arguments: {request: {action: describe, groupId: "${groupId}", reportId: "${reportId}"}}
    match: {request.action: describe, request.groupId: "${groupId}", request.reportId: "${reportId}"}
`)
	write(fixtureRoot, "ds/default.json", `{"status":"error","code":"unsupported_request"}`)
	write(fixtureRoot, "ds/primary.json", `{"status":"ok","data":[{"name":"primary","columns":[{"name":"channel","type":"string","role":"dimension","nullable":false},{"name":"units","type":"integer","role":"measure","format":"compactNumber","nullable":false}],"rows":[["Synthetic",12]],"hasMore":false}],"meta":{"source":"synthetic"}}`)

	mockServer, err := MockServer(Config{Folder: fixtureRoot, ReportRoot: catalogRoot, GroupID: "performance", ReportID: "channels"})
	if err != nil {
		t.Fatal(err)
	}
	mcpHost := httptest.NewServer(mockServer.HTTPHandler())
	defer mcpHost.Close()
	handler, err := Handler(Config{Folder: fixtureRoot, GroupID: "performance", ReportID: "channels", MCPURL: mcpHost.URL + "/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	previewHost := httptest.NewServer(handler)
	defer previewHost.Close()
	response, err := previewHost.Client().Get(previewHost.URL + "/api/compile?full=true")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !strings.Contains(string(body), `"title":"Channels"`) || !strings.Contains(string(body), `"channel":"Synthetic"`) {
		t.Fatalf("remote bootstrap failed: %d %.1000s", response.StatusCode, body)
	}
}

func TestProductionDescribeContractBootstrapsGenericRemoteReport(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("preview.yaml", `version: 1
groupId: advancedReporting
reportId: "400"
mcp:
  tool: AdvancedReportingRun
  defaultFile: default.json
  definition:
    tool: AdvancedReportingRun
    arguments: {request: {action: describe, groupId: "${groupId}", reportId: "${reportId}"}}
    match: {request.action: describe, request.groupId: "${groupId}", request.reportId: "${reportId}"}
`)
	write("ds/default.json", `{"status":"error","code":"unsupported_request"}`)
	write("ds/describe.json", `{"status":"ok","reports":[{"reportId":"400","viewName":"Channels & Devices","visualProfile":"mta_channels","fieldCatalog":{"reportId":"400","visualProfile":"mta_channels","columns":[{"name":"deviceChannelMix","type":"unknown","role":"dimension","nullable":true},{"name":"conversions","type":"unknown","role":"measure","format":"compactNumber","nullable":true}],"defaultDimensions":["deviceChannelMix"],"defaultMeasures":["conversions"],"allowedFilters":["advertiserId","campaignIds"],"resultSets":[{"name":"mtaChannelRanking","dimensions":["deviceChannelMix"],"measures":["conversions"],"fields":[{"name":"deviceChannelMix","role":"dimension"},{"name":"conversions","role":"measure"}]}]}}]}`)
	write("ds/run.json", `{"status":"ok","data":[{"name":"mtaChannelRanking","columns":[{"name":"deviceChannelMix","type":"string","role":"dimension","nullable":true},{"name":"conversions","type":"integer","role":"measure","format":"compactNumber","nullable":true}],"rows":[["SYN CTV / TV",42]],"hasMore":false}]}`)
	server, err := mock.New(mock.Config{Root: root, Tools: map[string]mock.Tool{
		"AdvancedReportingRun": {File: "default.json", Routes: []mock.Route{
			{Match: map[string]any{"request.action": "describe", "request.groupId": "advancedReporting", "request.reportId": "400"}, File: "describe.json"},
			{Match: map[string]any{"request.action": "run", "request.groupId": "advancedReporting", "request.reportId": "400", "request.resultSet": "mtaChannelRanking"}, File: "run.json"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	mcpHost := httptest.NewServer(server.HTTPHandler())
	defer mcpHost.Close()
	handler, err := Handler(Config{Folder: root, GroupID: "advancedReporting", ReportID: "400", MCPURL: mcpHost.URL + "/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	previewHost := httptest.NewServer(handler)
	defer previewHost.Close()
	response, err := previewHost.Client().Get(previewHost.URL + "/api/compile")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !strings.Contains(string(body), `"title":"Channels \u0026 Devices"`) || !strings.Contains(string(body), `"deviceChannelMix":"SYN CTV / TV"`) {
		t.Fatalf("production describe bootstrap failed: %d %.1500s", response.StatusCode, body)
	}
}

package preview

import (
	"encoding/json"
	"testing"

	"github.com/viant/agently/preview/datasource"
	"github.com/viant/forge/backend/types"
)

func TestDecodeRemoteDefinition(t *testing.T) {
	contract := &RemoteDefinition{
		Status: "ok", GroupID: "performance", ReportID: "channels",
		Endpoints:  map[string]datasource.Endpoint{"reports": {Type: "mcp", Transport: "streamable", BaseURL: "/mcp"}},
		Definition: Object{"kind": "ReportCatalogSelection", "report": Object{"id": "channels", "blocks": []any{}}},
		Extension:  Extension{Version: 1, DataSources: map[string]Declaration{"summary": {File: "summary.json"}}},
		Sources:    map[string]Source{"summary": {Service: &types.Service{Endpoint: "reports", URI: "ReportRun"}}},
		Datasets:   map[string]Dataset{"summary": {DataSource: "summary"}},
	}
	body, _ := json.Marshal(contract)
	p, err := DecodeRemoteDefinition(body, "performance", "channels", "", "ReportRun")
	if err != nil {
		t.Fatal(err)
	}
	if p.Variant != "default" || p.Sources["summary"].Service.URI != "ReportRun" {
		t.Fatalf("unexpected package: %+v", p)
	}
	if _, err = DecodeRemoteDefinition(body, "performance", "other", "default", "ReportRun"); err == nil {
		t.Fatal("identity mismatch was accepted")
	}
}

func TestDecodeProductionDescribeBuildsGenericRemotePackage(t *testing.T) {
	body := []byte(`{"status":"ok","reports":[{"reportId":"channels","viewName":"Channels","visualProfile":"channels","fieldCatalog":{"reportId":"channels","columns":[{"name":"channel","type":"unknown","role":"dimension","nullable":true},{"name":"units","type":"unknown","role":"measure","format":"compactNumber","nullable":true}],"defaultDimensions":["channel"],"defaultMeasures":["units"],"allowedFilters":["advertiserId","campaignIds"],"resultSets":[{"name":"channelSummary","dimensions":["channel"],"measures":["units"],"fields":[{"name":"channel","role":"dimension"},{"name":"units","role":"measure"}]}]}}]}`)
	p, err := DecodeRemoteDefinition(body, "performance", "channels", "", "ReportRun")
	if err != nil {
		t.Fatal(err)
	}
	if p.Extension.PrimaryDataSource != "channelSummary" || p.Sources["channelSummary"].MCPRequest == nil {
		t.Fatalf("production describe did not produce a runnable package: %+v", p)
	}
	request := p.Sources["channelSummary"].MCPRequest.Arguments["request"].(map[string]any)
	if request["action"] != "run" || request["groupId"] != "performance" || request["reportId"] != "channels" || request["resultSet"] != "channelSummary" {
		t.Fatalf("unexpected public MCP request template: %+v", request)
	}
	if p.Sources["channelSummary"].Columns[0]["type"] != "string" || p.Sources["channelSummary"].Columns[1]["type"] != "number" {
		t.Fatalf("unknown server column types were not safely normalized: %+v", p.Sources["channelSummary"].Columns)
	}
}

func TestMappedMCPRequestUsesProductionRequestShape(t *testing.T) {
	contract := &MCPRequest{Arguments: Object{"request": Object{"action": "run", "groupId": "performance", "reportId": "channels"}}, RequestPath: "request"}
	arguments, err := mappedMCPRequest(contract, Query{
		Projection: &Projection{Dimensions: []Field{{Field: "channel"}}, Measures: []Field{{Field: "units"}}},
		Filter:     &Predicate{And: []Predicate{{Field: "advertiserId", Op: "eq", Value: 7}, {Field: "options.mode", Op: "eq", Value: "current"}}},
		OrderBy:    []Order{{Field: "units", Direction: "desc"}},
		Page:       &Page{Limit: 25, Offset: 50},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := arguments["request"].(map[string]any)
	options := request["options"].(map[string]any)
	if request["advertiserId"] != 7 || options["mode"] != "current" || request["limit"] != 25 || request["offset"] != 50 {
		t.Fatalf("production request mapping lost parameters: %+v", request)
	}
	if got := request["dimensions"].([]string); len(got) != 1 || got[0] != "channel" {
		t.Fatalf("unexpected dimensions: %v", got)
	}
}

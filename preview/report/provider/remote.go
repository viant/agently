package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/viant/agently/preview/datasource"
	"github.com/viant/forge/backend/types"
)

// RemoteDefinition is the generic MCP response contract used to bootstrap a
// report preview without reading a local report catalog.
type RemoteDefinition struct {
	Status     string                         `json:"status"`
	GroupID    string                         `json:"groupId"`
	ReportID   string                         `json:"reportId"`
	Endpoints  map[string]datasource.Endpoint `json:"endpoints"`
	Definition Object                         `json:"definition"`
	Extension  Extension                      `json:"extension"`
	Sources    map[string]Source              `json:"sources"`
	Datasets   map[string]Dataset             `json:"datasets"`
}

func (p *Package) RemoteDefinition(groupID, reportID string) *RemoteDefinition {
	return &RemoteDefinition{Status: "ok", GroupID: groupID, ReportID: reportID, Endpoints: clone(p.Endpoints), Definition: clone(p.Definition), Extension: clone(p.Extension), Sources: clone(p.Sources), Datasets: clone(p.Datasets)}
}

// DescribeEnvelope mirrors a production report-server describe response while
// carrying the authored preview package as an optional extension.
func (p *Package) DescribeEnvelope(groupID, reportID string) Object {
	registry, _ := p.Definition["registry"].(map[string]any)
	metadata, _ := p.Definition["metadata"].(map[string]any)
	report := clone(registry)
	if report == nil {
		report = Object{}
	}
	report["reportId"] = reportID
	if text(report["viewName"]) == "" {
		report["viewName"] = metadata["title"]
	}
	columnsByName := map[string]Object{}
	resultSets := make([]any, 0, len(p.Sources))
	for _, id := range sortedKeys(p.Sources) {
		source := p.Sources[id]
		for _, column := range source.Columns {
			columnsByName[text(column["name"])] = clone(column)
		}
		declaration := p.Extension.DataSources[id]
		fields := make([]any, 0, len(source.Columns))
		for _, column := range source.Columns {
			fields = append(fields, clone(column))
		}
		resultSets = append(resultSets, Object{"name": id, "dimensions": append([]string{}, declaration.Dimensions...), "measures": sortedKeys(declaration.Measures), "fields": fields})
	}
	columnNames := make([]string, 0, len(columnsByName))
	for name := range columnsByName {
		columnNames = append(columnNames, name)
	}
	sort.Strings(columnNames)
	columns := make([]any, 0, len(columnNames))
	for _, name := range columnNames {
		columns = append(columns, columnsByName[name])
	}
	fieldCatalog := Object{"reportId": reportID, "visualProfile": registry["visualProfile"], "columns": columns, "defaultDimensions": registry["defaultDimensions"], "defaultMeasures": registry["defaultMeasures"], "defaultKpis": registry["defaultKpis"], "allowedFilters": registry["allowedFilters"], "allowedSorts": registry["allowedSorts"], "options": registry["options"], "resultSets": resultSets}
	report["fieldCatalog"], report["resultSets"] = fieldCatalog, resultSets
	return Object{"status": "ok", "reports": []any{report}, "previewDefinition": p.RemoteDefinition(groupID, reportID)}
}

func DecodeRemoteDefinition(body []byte, groupID, reportID, variant, tool string) (*Package, error) {
	var envelope Object
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode remote report definition: %w", err)
	}
	if extension, ok := envelope["previewDefinition"].(map[string]any); ok {
		encoded, _ := json.Marshal(extension)
		return decodePreviewDefinition(encoded, groupID, reportID, variant)
	}
	if envelope["definition"] == nil && envelope["reports"] != nil {
		return decodeDescribeDefinition(envelope, groupID, reportID, variant, tool)
	}
	return decodePreviewDefinition(body, groupID, reportID, variant)
}

func decodePreviewDefinition(body []byte, groupID, reportID, variant string) (*Package, error) {
	if variant == "" {
		variant = "default"
	}
	contract := &RemoteDefinition{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(contract); err != nil {
		return nil, fmt.Errorf("decode remote report definition: %w", err)
	}
	if contract.Status != "ok" || contract.GroupID != groupID || contract.ReportID != reportID {
		return nil, fmt.Errorf("remote report definition identity mismatch")
	}
	if contract.Definition == nil || len(contract.Sources) == 0 || len(contract.Datasets) == 0 {
		return nil, fmt.Errorf("remote report definition is incomplete")
	}
	limits, err := (Limits{}).normalized()
	if err != nil {
		return nil, err
	}
	if contract.Extension.MaxPageSize <= 0 {
		contract.Extension.MaxPageSize = limits.PageSize
	}
	p := &Package{Endpoints: contract.Endpoints, Definition: contract.Definition, Extension: contract.Extension, Sources: contract.Sources, Datasets: contract.Datasets, Variant: variant, fixtures: map[string]*fixture{}, limits: limits, queries: make(chan struct{}, limits.ConcurrentQueries)}
	identity, _ := json.Marshal(contract)
	p.definitionHash = hash(identity)
	return p, nil
}

func decodeDescribeDefinition(envelope Object, groupID, reportID, variant, tool string) (*Package, error) {
	if text(envelope["status"]) != "ok" {
		return nil, fmt.Errorf("remote report describe failed")
	}
	reports, _ := envelope["reports"].([]any)
	if len(reports) != 1 {
		return nil, fmt.Errorf("remote report describe must return exactly one report")
	}
	report, _ := reports[0].(map[string]any)
	if report == nil || strings.TrimSpace(text(report["reportId"])) != reportID {
		return nil, fmt.Errorf("remote report definition identity mismatch")
	}
	if tool = strings.TrimSpace(tool); tool == "" {
		return nil, fmt.Errorf("remote report definition requires an MCP tool name")
	}
	catalog, _ := report["fieldCatalog"].(map[string]any)
	if catalog == nil {
		catalog = report
	}
	title := strings.TrimSpace(text(report["viewName"]))
	if title == "" {
		title = "Report " + reportID
	}
	resultSets := remoteResultSets(report, catalog)
	columns := normalizeRemoteColumns(objects(catalog["columns"]))
	if len(resultSets) == 0 {
		resultSets = []remoteResultSet{{Name: "primary", Columns: columns, Dimensions: stringsOf(catalog["defaultDimensions"]), Measures: stringsOf(catalog["defaultMeasures"])}}
	}
	profile := genericRemoteProfile(resultSets, title)
	blocks, layout, datasetIDs, err := compileCatalogProfile(profile, title)
	if err != nil {
		return nil, err
	}
	parameters, bindings, err := remoteParameters(catalog)
	if err != nil {
		return nil, err
	}
	limits, err := (Limits{}).normalized()
	if err != nil {
		return nil, err
	}
	if variant == "" {
		variant = "default"
	}
	p := &Package{
		Endpoints: map[string]datasource.Endpoint{"mockReports": {Type: "mcp", Transport: "streamable", BaseURL: "/mcp"}},
		Definition: Object{
			"apiVersion": "reporting.viant.ai/v1alpha1", "kind": "RemoteReportCatalogSelection",
			"metadata": Object{"id": reportID, "title": title, "visualProfile": catalog["visualProfile"], "definitionSource": "mcp-describe"},
			"registry": report, "report": Object{"id": reportID, "title": title, "blocks": blocks, "layout": layout}, "parameters": parameters,
		},
		Extension: Extension{Version: 1, DataRoot: "", DefaultVariant: "default", MaxPageSize: limits.PageSize, DataSources: map[string]Declaration{}},
		Sources:   map[string]Source{}, Datasets: map[string]Dataset{}, Variant: variant,
		fixtures: map[string]*fixture{}, limits: limits, queries: make(chan struct{}, limits.ConcurrentQueries),
	}
	byName := map[string]remoteResultSet{}
	for _, resultSet := range resultSets {
		byName[resultSet.Name] = resultSet
	}
	for _, id := range datasetIDs {
		resultSet := byName[id]
		declaration := Declaration{ResultName: id, InputContract: Contract{Shape: "tabular", ResultsPath: "data", ResultName: id}, Query: Capabilities{Projection: true, Filtering: true, Sorting: true, Pagination: "offset"}, Measures: map[string]Measure{}}
		for _, column := range resultSet.Columns {
			name, role := text(column["name"]), text(column["role"])
			if role == "dimension" {
				declaration.Dimensions = append(declaration.Dimensions, name)
			} else if role == "measure" {
				declaration.Measures[name] = Measure{Aggregation: "none", RowLevel: true}
			}
		}
		request := Object{"action": "run", "groupId": groupID, "reportId": reportID}
		if id != "primary" {
			request["resultSet"] = id
		}
		p.Extension.DataSources[id] = declaration
		p.Sources[id] = Source{Service: &types.Service{Endpoint: "mockReports", URI: tool, Method: "POST"}, Columns: resultSet.Columns, ResultContract: declaration.InputContract, ParameterBindings: clone(bindings), MCPRequest: &MCPRequest{Arguments: Object{"request": request}, RequestPath: "request"}}
		projection := &Projection{Dimensions: toFields(resultSet.Dimensions), Measures: toFields(resultSet.Measures)}
		if len(projection.Dimensions) == 0 && len(projection.Measures) == 0 {
			projection = nil
		}
		p.Datasets[id] = Dataset{DataSource: id, Query: Query{Projection: projection, Page: &Page{Limit: min(100, limits.PageSize)}}}
	}
	if len(datasetIDs) > 0 {
		p.Extension.PrimaryDataSource = datasetIDs[0]
	}
	identity, _ := json.Marshal(report)
	p.definitionHash = hash(identity)
	return p, nil
}

type remoteResultSet struct {
	Name       string
	Columns    []Object
	Dimensions []string
	Measures   []string
}

func remoteResultSets(report, catalog Object) []remoteResultSet {
	raw := objects(report["resultSets"])
	if len(raw) == 0 {
		raw = objects(catalog["resultSets"])
	}
	result := make([]remoteResultSet, 0, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(text(item["name"]))
		if name == "" {
			continue
		}
		dimensions, measures := stringsOf(item["dimensions"]), stringsOf(item["measures"])
		fields := normalizeRemoteColumns(objects(item["fields"]))
		if len(fields) == 0 {
			for _, field := range dimensions {
				fields = append(fields, Object{"name": field, "type": "string", "role": "dimension", "nullable": true})
			}
			for _, field := range measures {
				fields = append(fields, Object{"name": field, "type": "number", "role": "measure", "nullable": true})
			}
		}
		result = append(result, remoteResultSet{Name: name, Columns: fields, Dimensions: dimensions, Measures: measures})
	}
	return result
}

func normalizeRemoteColumns(source []Object) []Object {
	result := make([]Object, 0, len(source))
	for _, raw := range source {
		name, role := strings.TrimSpace(text(raw["name"])), strings.TrimSpace(text(raw["role"]))
		if name == "" || (role != "dimension" && role != "measure") {
			continue
		}
		column := clone(raw)
		column["name"], column["role"], column["nullable"] = name, role, true
		if kind := strings.TrimSpace(text(column["type"])); kind == "" || kind == "unknown" {
			if role == "dimension" {
				column["type"] = "string"
			} else {
				column["type"] = "number"
			}
		}
		delete(column, "sqlName")
		result = append(result, column)
	}
	return result
}

func genericRemoteProfile(resultSets []remoteResultSet, title string) Object {
	tabs, blocks := []any{}, []any{}
	for index, resultSet := range resultSets {
		sectionID := fmt.Sprintf("remoteSection%d", index+1)
		tableID := fmt.Sprintf("remoteTable%d", index+1)
		blockIDs := []any{}
		if len(resultSet.Dimensions) > 0 && len(resultSet.Measures) > 0 {
			chartID := fmt.Sprintf("remoteChart%d", index+1)
			chartType := "horizontal_bar"
			if strings.Contains(strings.ToLower(resultSet.Dimensions[0]), "date") || strings.Contains(strings.ToLower(resultSet.Dimensions[0]), "time") {
				chartType = "line"
			}
			blocks = append(blocks, Object{"id": chartID, "kind": "chartBlock", "title": remoteLabel(resultSet.Name), "datasetRef": resultSet.Name, "chartSpec": Object{"type": chartType, "xField": resultSet.Dimensions[0], "yFields": []any{resultSet.Measures[0]}}, "rowLimit": 50})
			blockIDs = append(blockIDs, chartID)
		}
		columns := make([]any, 0, len(resultSet.Columns))
		for _, column := range resultSet.Columns {
			item := Object{"key": column["name"], "label": remoteLabel(text(column["name"]))}
			if column["format"] != nil {
				item["format"] = column["format"]
			}
			columns = append(columns, item)
		}
		blocks = append(blocks, Object{"id": tableID, "kind": "tableBlock", "title": remoteLabel(resultSet.Name) + " evidence", "datasetRef": resultSet.Name, "columns": columns})
		blockIDs = append(blockIDs, tableID)
		tabs = append(tabs, Object{"id": sectionID, "title": remoteLabel(resultSet.Name), "blockIds": blockIDs})
	}
	return Object{"tabs": tabs, "blocks": blocks, "title": title}
}

func remoteLabel(value string) string {
	value = strings.NewReplacer("_", " ", "-", " ").Replace(strings.TrimSpace(value))
	var result strings.Builder
	for index, current := range value {
		if index > 0 && current >= 'A' && current <= 'Z' {
			result.WriteByte(' ')
		}
		result.WriteRune(current)
	}
	words := strings.Fields(result.String())
	for index, word := range words {
		if word != "" {
			words[index] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func remoteParameters(catalog Object) ([]any, []Binding, error) {
	parameters, err := catalogParameters(catalog["options"])
	if err != nil {
		return nil, nil, err
	}
	bindings := []Binding{}
	seen := map[string]bool{}
	for _, value := range parameters {
		param, _ := value.(map[string]any)
		name := text(param["name"])
		seen[name] = true
		bindings = append(bindings, Binding{Parameter: name, Field: "options." + name, Operator: "eq"})
	}
	for _, scope := range []Object{{"name": "advertiserId", "type": "integer"}, {"name": "suiteId", "type": "integer"}, {"name": "trial", "type": "boolean", "default": false}} {
		name := text(scope["name"])
		if !seen[name] {
			parameters = append(parameters, scope)
			bindings = append(bindings, Binding{Parameter: name, Field: name, Operator: "eq"})
			seen[name] = true
		}
	}
	for _, name := range stringsOf(catalog["allowedFilters"]) {
		if seen[name] {
			continue
		}
		param := Object{"name": name, "type": "string"}
		if strings.HasSuffix(name, "Ids") {
			param["type"], param["multiple"] = "integer", true
		} else if strings.HasSuffix(name, "Id") {
			param["type"] = "integer"
		}
		parameters = append(parameters, param)
		bindings = append(bindings, Binding{Parameter: name, Field: name, Operator: "eq"})
	}
	return parameters, bindings, nil
}

func objects(value any) []Object {
	raw, _ := value.([]any)
	result := make([]Object, 0, len(raw))
	for _, item := range raw {
		if object, ok := item.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func toFields(names []string) []Field {
	result := make([]Field, 0, len(names))
	for _, name := range names {
		result = append(result, Field{Field: name})
	}
	return result
}

package preview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/viant/agently/preview/datasource"
	"github.com/viant/forge/backend/types"
	"gopkg.in/yaml.v3"
)

// CatalogOptions selects an existing Forge reporting definition/profile while
// keeping preview data in a separate fixture directory.
type CatalogOptions struct {
	ReportRoot  string
	FixtureRoot string
	GroupID     string
	ReportID    string
	Variant     string
}

type catalogBuilder struct {
	ReportBuilder struct {
		PresentationProfileRefs []string `yaml:"presentationProfileRefs"`
	} `yaml:"reportBuilder"`
}

type catalogPreviewManifest struct {
	Version  int    `yaml:"version"`
	GroupID  string `yaml:"groupId"`
	ReportID string `yaml:"reportId"`
	MCP      struct {
		Tool string `yaml:"tool"`
	} `yaml:"mcp"`
	Bindings []struct {
		Parameter  string `yaml:"parameter"`
		DataSource string `yaml:"dataSource"`
		Field      string `yaml:"field"`
		Operator   string `yaml:"operator"`
	} `yaml:"bindings"`
}

// ListCatalog returns compact report identities from one validated Forge report group.
func ListCatalog(reportRoot, groupID string) ([]Object, error) {
	root, err := filepath.Abs(reportRoot)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	groupBody, err := os.ReadFile(filepath.Join(root, "group.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load report group: %w", err)
	}
	var group struct {
		Kind string `yaml:"kind"`
		ID   string `yaml:"id"`
	}
	if err = yaml.Unmarshal(groupBody, &group); err != nil {
		return nil, fmt.Errorf("decode report group: %w", err)
	}
	if group.Kind != "forge.reporting.group" || group.ID != groupID {
		return nil, fmt.Errorf("report group %q is not available from this catalog", groupID)
	}
	body, err := os.ReadFile(filepath.Join(root, "definitions.json"))
	if err != nil {
		return nil, fmt.Errorf("load report catalog definitions: %w", err)
	}
	var catalog struct {
		Views []Object `json:"views"`
	}
	if err = json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("decode report catalog definitions: %w", err)
	}
	result := make([]Object, 0, len(catalog.Views))
	for _, definition := range catalog.Views {
		id := catalogReportID(definition)
		if id == "" {
			return nil, fmt.Errorf("report catalog entry is missing reportId")
		}
		item := Object{"groupId": groupID, "reportId": id, "title": definition["name"], "visualProfile": definition["visualProfile"]}
		if reason := definition["unavailableReason"]; reason != nil {
			item["unavailableReason"] = reason
		}
		result = append(result, item)
	}
	return result, nil
}

func LoadCatalog(options CatalogOptions) (*Package, error) {
	if strings.TrimSpace(options.GroupID) == "" {
		return nil, fmt.Errorf("report catalog preview requires a group ID")
	}
	if strings.TrimSpace(options.ReportID) == "" {
		return nil, fmt.Errorf("report catalog preview requires a report ID")
	}
	reportingRoot, err := filepath.Abs(options.ReportRoot)
	if err != nil {
		return nil, err
	}
	reportingRoot, err = filepath.EvalSymlinks(reportingRoot)
	if err != nil {
		return nil, err
	}
	groupBody, err := os.ReadFile(filepath.Join(reportingRoot, "group.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load report group: %w", err)
	}
	var group struct {
		Kind string `yaml:"kind"`
		ID   string `yaml:"id"`
	}
	if err = yaml.Unmarshal(groupBody, &group); err != nil {
		return nil, fmt.Errorf("decode report group: %w", err)
	}
	if group.Kind != "forge.reporting.group" || group.ID != options.GroupID {
		return nil, fmt.Errorf("report group %q is not available from this catalog", options.GroupID)
	}
	fixtureRoot, err := filepath.Abs(options.FixtureRoot)
	if err != nil {
		return nil, err
	}
	fixtureRoot, err = filepath.EvalSymlinks(fixtureRoot)
	if err != nil {
		return nil, err
	}
	manifestBody, err := os.ReadFile(filepath.Join(fixtureRoot, "preview.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load report preview manifest: %w", err)
	}
	var manifest catalogPreviewManifest
	if err = yaml.Unmarshal(manifestBody, &manifest); err != nil {
		return nil, fmt.Errorf("decode report preview manifest: %w", err)
	}
	if manifest.Version != 1 || strings.TrimSpace(manifest.MCP.Tool) == "" {
		return nil, fmt.Errorf("report preview manifest requires version 1 and mcp.tool")
	}
	if manifest.ReportID != "" && manifest.ReportID != options.ReportID {
		return nil, fmt.Errorf("report ID %q does not match preview manifest %q", options.ReportID, manifest.ReportID)
	}
	if manifest.GroupID != "" && manifest.GroupID != options.GroupID {
		return nil, fmt.Errorf("group ID %q does not match preview manifest %q", options.GroupID, manifest.GroupID)
	}
	limits, err := (Limits{}).normalized()
	if err != nil {
		return nil, err
	}

	definitionBytes, err := os.ReadFile(filepath.Join(reportingRoot, "definitions.json"))
	if err != nil {
		return nil, fmt.Errorf("load report catalog definitions: %w", err)
	}
	var definitions struct {
		Views []Object `json:"views"`
	}
	if err = json.Unmarshal(definitionBytes, &definitions); err != nil {
		return nil, fmt.Errorf("decode report catalog definitions: %w", err)
	}
	var definition Object
	for _, candidate := range definitions.Views {
		if catalogReportID(candidate) == options.ReportID {
			definition = candidate
			break
		}
	}
	if definition == nil {
		return nil, fmt.Errorf("report %q is absent from catalog definitions", options.ReportID)
	}
	parameters, err := catalogParameters(definition["options"])
	if err != nil {
		return nil, fmt.Errorf("report %q options: %w", options.ReportID, err)
	}

	builderBytes, err := os.ReadFile(filepath.Join(reportingRoot, "builder.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load report catalog builder: %w", err)
	}
	var builder catalogBuilder
	if err = yaml.Unmarshal(builderBytes, &builder); err != nil {
		return nil, fmt.Errorf("decode report catalog builder: %w", err)
	}
	var profile Object
	for _, relative := range builder.ReportBuilder.PresentationProfileRefs {
		path, pathErr := confinedPath(reportingRoot, relative)
		if pathErr != nil {
			return nil, pathErr
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		var catalog struct {
			Kind          string   `json:"kind"`
			SchemaVersion int      `json:"schemaVersion"`
			Views         []Object `json:"views"`
		}
		if json.Unmarshal(body, &catalog) != nil || catalog.Kind != "forge.reporting.presentationProfileCatalog" || catalog.SchemaVersion != 1 {
			continue
		}
		for _, candidate := range catalog.Views {
			if catalogReportID(candidate) == options.ReportID && strings.TrimSpace(text(candidate["visualProfile"])) == strings.TrimSpace(text(definition["visualProfile"])) {
				profile = candidate
				break
			}
		}
		if profile != nil {
			break
		}
	}
	if profile == nil {
		return nil, fmt.Errorf("report %q profile %q is absent from catalog profile references", options.ReportID, text(definition["visualProfile"]))
	}

	title := strings.TrimSpace(text(definition["name"]))
	if title == "" {
		title = "Report " + options.ReportID
	}
	blocks, layout, datasetIDs, err := compileCatalogProfile(profile, title)
	if err != nil {
		return nil, err
	}
	variant := strings.TrimSpace(options.Variant)
	if variant == "" {
		variant = "default"
	}
	if !safeID.MatchString(variant) {
		return nil, fail("PathEscape", "", "variant", "unsafe variant")
	}

	p := &Package{
		Endpoints: map[string]datasource.Endpoint{"mockReports": {Type: "mcp", Transport: "streamable", BaseURL: "/mcp"}},
		Definition: Object{
			"apiVersion": "reporting.viant.ai/v1alpha1",
			"kind":       "ReportCatalogSelection",
			"metadata": Object{
				"id":               options.ReportID,
				"title":            title,
				"revision":         profile["revision"],
				"visualProfile":    definition["visualProfile"],
				"definitionSource": "catalog",
			},
			"registry": definition,
			"report": Object{
				"id":       options.ReportID,
				"title":    title,
				"subtitle": profile["subtitle"],
				"blocks":   blocks,
				"layout":   layout,
			},
			"parameters": parameters,
		},
		Extension: Extension{Version: 1, DataRoot: "ds", DefaultVariant: "default", MaxPageSize: limits.PageSize, DataSources: map[string]Declaration{}},
		Sources:   map[string]Source{}, Datasets: map[string]Dataset{}, Variant: variant,
		fixtures: map[string]*fixture{}, limits: limits, queries: make(chan struct{}, limits.ConcurrentQueries),
	}
	identity, _ := json.Marshal([]any{definition, profile})
	p.definitionHash = hash(identity)
	for _, id := range datasetIDs {
		body, loadErr := readCatalogFixture(fixtureRoot, variant, id, limits.FixtureBytes)
		if loadErr != nil {
			return nil, fmt.Errorf("catalog report %q datasource %s: %w", options.ReportID, id, loadErr)
		}
		declaration := Declaration{
			File: id + ".json", ResultName: id,
			InputContract: Contract{Shape: "tabular", ResultsPath: "data", ResultName: id},
			Query:         Capabilities{Projection: true, Filtering: true, Sorting: true, Pagination: "offset"},
			Measures:      map[string]Measure{},
		}
		decoded, decodeErr := decodeFixture(id, body, declaration, Source{})
		if decodeErr != nil {
			return nil, decodeErr
		}
		for _, column := range decoded.columns {
			name, role := text(column["name"]), text(column["role"])
			if role == "dimension" {
				declaration.Dimensions = append(declaration.Dimensions, name)
			} else if role == "measure" {
				declaration.Measures[name] = Measure{Aggregation: "none", RowLevel: true}
			}
		}
		p.Extension.DataSources[id] = declaration
		p.Sources[id] = Source{
			Service:        &types.Service{Endpoint: "mockReports", URI: manifest.MCP.Tool, Method: "POST"},
			Columns:        decoded.columns,
			ResultContract: Contract{Shape: "tabular", ResultsPath: "data", ResultName: id},
		}
		for _, binding := range manifest.Bindings {
			if binding.DataSource == id {
				operator := binding.Operator
				if operator == "" {
					operator = "eq"
				}
				source := p.Sources[id]
				source.ParameterBindings = append(source.ParameterBindings, Binding{Parameter: binding.Parameter, Field: binding.Field, Operator: operator})
				p.Sources[id] = source
			}
		}
		p.Datasets[id] = Dataset{DataSource: id}
		p.fixtures[id] = decoded
	}
	if len(datasetIDs) > 0 {
		p.Extension.PrimaryDataSource = datasetIDs[0]
	}
	applyCatalogChartFormats(blocks, p.fixtures)
	if err = validateCatalogProfileData(blocks, p.fixtures); err != nil {
		return nil, err
	}
	return p, nil
}

func applyCatalogChartFormats(blocks []any, fixtures map[string]*fixture) {
	for _, value := range blocks {
		block, _ := value.(map[string]any)
		if text(block["kind"]) != "chartBlock" {
			continue
		}
		fixture := fixtures[text(block["datasetRef"])]
		chart, _ := block["chartSpec"].(map[string]any)
		if fixture == nil || chart == nil {
			continue
		}
		formats := map[string]string{}
		for _, column := range fixture.columns {
			if format := text(column["format"]); format != "" {
				formats[text(column["name"])] = format
			}
		}
		options, _ := chart["seriesOptions"].(map[string]any)
		if options == nil {
			options = map[string]any{}
		}
		for _, field := range stringsOf(chart["yFields"]) {
			if formats[field] == "" {
				continue
			}
			entry, _ := options[field].(map[string]any)
			if entry == nil {
				entry = map[string]any{}
			}
			if text(entry["format"]) == "" {
				entry["format"] = formats[field]
			}
			options[field] = entry
		}
		chart["seriesOptions"] = options
	}
}

func catalogParameters(raw any) ([]any, error) {
	options, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	result := make([]any, 0, len(options))
	for _, value := range options {
		option, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("option must be an object")
		}
		name := strings.TrimSpace(text(option["name"]))
		if name == "" {
			return nil, fmt.Errorf("option requires name")
		}
		parameter := Object{"name": name, "type": text(option["type"]), "default": option["default"]}
		if values, ok := option["values"].([]any); ok {
			parameter["values"] = values
		}
		if text(option["type"]) == "integer" {
			if option["default"] != nil {
				parameter["default"] = float64(integer(option["default"]))
			}
			if values, ok := parameter["values"].([]any); ok {
				converted := make([]any, len(values))
				for i, value := range values {
					converted[i] = float64(integer(value))
				}
				parameter["values"] = converted
			}
		}
		result = append(result, parameter)
	}
	return result, nil
}

func confinedPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("catalog profile reference must be relative: %s", relative)
	}
	path, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("catalog profile reference escapes report root: %s", relative)
	}
	return path, nil
}

func readCatalogFixture(root, variant, id string, limit int64) ([]byte, error) {
	if !safeID.MatchString(id) {
		return nil, fmt.Errorf("unsafe catalog datasource ID %q", id)
	}
	base := filepath.Join(root, "ds", id+".json")
	path := base
	if variant != "default" {
		overlay := filepath.Join(root, "variants", variant, "ds", id+".json")
		if _, err := os.Stat(overlay); err == nil {
			path = overlay
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("fixture exceeds size limit")
	}
	return body, nil
}

func compileCatalogProfile(profile Object, title string) ([]any, Object, []string, error) {
	rawBlocks, ok := profile["blocks"].([]any)
	if !ok {
		return nil, nil, nil, fmt.Errorf("catalog presentation profile requires blocks")
	}
	rawTabs, ok := profile["tabs"].([]any)
	if !ok {
		return nil, nil, nil, fmt.Errorf("catalog presentation profile requires tabs")
	}
	blockByID := map[string]Object{}
	compositeChildren := map[string]bool{}
	for _, value := range rawBlocks {
		raw, ok := value.(map[string]any)
		if !ok {
			return nil, nil, nil, fmt.Errorf("catalog profile block must be an object")
		}
		block := clone(Object(raw))
		id := strings.TrimSpace(text(block["id"]))
		if id == "" {
			return nil, nil, nil, fmt.Errorf("catalog profile block requires id")
		}
		delete(block, "requiresFields")
		delete(block, "collapsedPreviewRows")
		if text(block["kind"]) == "kpiBlock" {
			if text(block["valueLabel"]) == "" {
				block["valueLabel"] = block["title"]
			}
			if text(block["emptyLabel"]) == "" {
				block["emptyLabel"] = "Unavailable"
			}
		}
		if text(block["kind"]) == "chartBlock" {
			chart, _ := block["chartSpec"].(map[string]any)
			if chart == nil {
				chart = Object{}
			}
			if text(chart["accentTone"]) == "" {
				chart["accentTone"] = "blue"
			}
			block["chartSpec"] = chart
		}
		if text(block["kind"]) == "tableBlock" {
			if block["collapsible"] == nil {
				block["collapsible"] = true
			}
			if block["defaultCollapsed"] == nil {
				block["defaultCollapsed"] = true
			}
			if text(block["accentTone"]) == "" {
				block["accentTone"] = "blue"
			}
		}
		blockByID[id] = block
		if text(block["kind"]) == "compositeBlock" {
			for _, child := range stringsOf(block["childBlockIds"]) {
				compositeChildren[child] = true
			}
		}
	}

	sectionIDs := []string{}
	blocks := []any{}
	layoutItems := []any{}
	tabGroup := Object{"id": "advancedPresentationTabs", "kind": "tabGroupBlock", "title": title, "sectionIds": sectionIDs, "defaultSectionId": "", "includeUnlistedSections": false}
	blocks = append(blocks, tabGroup)
	layoutItems = append(layoutItems, Object{"blockId": "advancedPresentationTabs"})
	emitted := map[string]bool{}
	dataSources := map[string]bool{}
	var appendBlock func(string)
	appendBlock = func(id string) {
		if emitted[id] || blockByID[id] == nil {
			return
		}
		emitted[id] = true
		block := blockByID[id]
		blocks = append(blocks, block)
		if ref := strings.TrimSpace(text(block["datasetRef"])); ref != "" {
			dataSources[ref] = true
		}
		if text(block["kind"]) == "compositeBlock" {
			for _, child := range stringsOf(block["childBlockIds"]) {
				appendBlock(child)
			}
		}
	}
	for _, value := range rawTabs {
		tab, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(text(tab["id"]))
		owned := []string{}
		for _, blockID := range stringsOf(tab["blockIds"]) {
			if blockByID[blockID] != nil {
				owned = append(owned, blockID)
			}
		}
		if id == "" || len(owned) == 0 {
			continue
		}
		sectionIDs = append(sectionIDs, id)
		section := Object{"id": id, "kind": "sectionBlock", "title": strings.TrimSpace(text(tab["title"])), "navigationLabel": strings.TrimSpace(text(tab["title"])), "blockIds": owned}
		if subtitle := strings.TrimSpace(text(tab["subtitle"])); subtitle != "" {
			section["subtitle"] = subtitle
		}
		blocks = append(blocks, section)
		layoutItems = append(layoutItems, Object{"blockId": id})
		for _, blockID := range owned {
			appendBlock(blockID)
			if !compositeChildren[blockID] {
				layoutItems = append(layoutItems, Object{"blockId": blockID})
			}
		}
	}
	if len(sectionIDs) == 0 {
		return nil, nil, nil, fmt.Errorf("catalog profile produced no report sections")
	}
	tabGroup["sectionIds"] = sectionIDs
	tabGroup["defaultSectionId"] = sectionIDs[0]
	ids := make([]string, 0, len(dataSources))
	for id := range dataSources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return blocks, Object{"type": "stack", "items": layoutItems}, ids, nil
}

func validateCatalogProfileData(blocks []any, fixtures map[string]*fixture) error {
	for _, value := range blocks {
		block, _ := value.(map[string]any)
		ref := strings.TrimSpace(text(block["datasetRef"]))
		if ref == "" {
			continue
		}
		fixture := fixtures[ref]
		if fixture == nil {
			return fmt.Errorf("catalog profile block %s requires missing datasource fixture %s.json", text(block["id"]), ref)
		}
		if fixture.errorResponse {
			continue
		}
		available := map[string]bool{}
		for _, column := range fixture.columns {
			available[text(column["name"])] = true
		}
		needed := []string{}
		for _, key := range []string{"valueField", "secondaryField"} {
			if field := text(block[key]); field != "" {
				needed = append(needed, field)
			}
		}
		if chart, ok := block["chartSpec"].(map[string]any); ok {
			needed = append(needed, text(chart["xField"]))
			needed = append(needed, stringsOf(chart["yFields"])...)
		}
		if columns, ok := block["columns"].([]any); ok {
			for _, item := range columns {
				column, _ := item.(map[string]any)
				needed = append(needed, text(column["key"]))
			}
		}
		for _, field := range needed {
			if field != "" && !available[field] {
				return fmt.Errorf("catalog profile block %s requires datasource %s field %s", text(block["id"]), ref, field)
			}
		}
	}
	return nil
}

func catalogReportID(entry Object) string {
	return strings.TrimSpace(text(entry["reportId"]))
}

func text(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func integer(value any) int {
	switch actual := value.(type) {
	case float64:
		return int(actual)
	case int:
		return actual
	case json.Number:
		result, _ := strconv.Atoi(actual.String())
		return result
	default:
		result, _ := strconv.Atoi(fmt.Sprint(value))
		return result
	}
}
func stringsOf(value any) []string {
	result := []string{}
	switch values := value.(type) {
	case []any:
		for _, item := range values {
			result = append(result, strings.TrimSpace(fmt.Sprint(item)))
		}
	case []string:
		for _, item := range values {
			result = append(result, strings.TrimSpace(item))
		}
	}
	return result
}

package preview

import (
	"encoding/json"
	"fmt"
	forgepdf "github.com/viant/forge/backend/reporting/export/pdf"
	"github.com/viant/forge/backend/reporting/fenced"
	reportprint "github.com/viant/forge/backend/reporting/print"
	"time"
)

type Compiled struct {
	*fenced.CompileResult
	Results  map[string]*Result `json:"results"`
	Variant  string             `json:"variant"`
	Warnings []string           `json:"warnings,omitempty"`
}

// Compile resolves datasets through validated queries before invoking Forge's
// report-document-v1 compiler. Full mode drains pages independently of UI state.
func (p *Package) Compile(parameters Object, full bool) (*Compiled, error) {
	parameters, err := p.resolveParameters(parameters)
	if err != nil {
		return nil, err
	}
	results := map[string]*Result{}
	warnings := append([]string{}, p.Warnings...)
	for _, id := range sortedKeys(p.Datasets) {
		ds := p.Datasets[id]
		q := clone(ds.Query)
		for _, binding := range p.Sources[ds.DataSource].ParameterBindings {
			if value, ok := parameters[binding.Parameter]; ok {
				leaf := Predicate{Field: binding.Field, Op: binding.Operator, Value: value}
				if q.Filter == nil {
					q.Filter = &leaf
				} else {
					q.Filter = &Predicate{And: []Predicate{*q.Filter, leaf}}
				}
			}
		}
		r, err := p.Execute(ds.DataSource, q)
		if err != nil {
			return nil, err
		}
		if isErrorResponse(r.Response) {
			if !p.Extension.DataSources[ds.DataSource].Optional {
				return nil, fail("DataSourceError", ds.DataSource, "", "required datasource returned an error")
			}
			warnings = append(warnings, "optional datasource failed: "+ds.DataSource)
			results[id] = r
			continue
		}
		if full {
			if p.executeRemote == nil && p.fixtures[ds.DataSource].hasMore {
				return nil, fail("IncompleteData", ds.DataSource, "", "fixture is partial; full export cannot be produced")
			}
			if p.Extension.DataSources[ds.DataSource].Query.Pagination == "offset" {
				q.Page = &Page{Limit: p.Extension.MaxPageSize}
				r, err = p.Execute(ds.DataSource, q)
				if err != nil {
					return nil, err
				}
				all := append([]Object{}, r.Rows...)
				for r.HasMore {
					q.Page.Offset += q.Page.Limit
					next, e := p.Execute(ds.DataSource, q)
					if e != nil {
						return nil, e
					}
					if len(next.Rows) == 0 {
						return nil, fail("IncompleteData", ds.DataSource, "", "provider reported more rows without progress")
					}
					all = append(all, next.Rows...)
					r.HasMore = next.HasMore
				}
				r.Rows = all
				r.Response = encodeResponse(r.Response, p.Sources[ds.DataSource].ResultContract, p.Extension.DataSources[ds.DataSource], r.Columns, all, false, len(all))
			} else if r.HasMore {
				return nil, fail("UnsupportedCapability", ds.DataSource, "pagination", "full result requires pagination")
			}
		}
		results[id] = r
	}
	source, err := p.forgeSource(results)
	if err != nil {
		return nil, err
	}
	reportID := str(source["id"])
	if reportID == "" {
		reportID = "preview"
	}
	seq := 1
	fences := []fenced.Fence{}
	add := func(kind string, obj Object) {
		obj["sequence"] = seq
		seq++
		b, _ := json.Marshal(obj)
		fences = append(fences, fenced.Fence{Kind: kind, Payload: b})
	}
	source["version"] = 1
	source["scope"] = "message"
	source["id"] = reportID
	source["mode"] = "start"
	source["grammar"] = "report-document-v1"
	add(fenced.ReportFence, source)
	for _, id := range sortedKeys(results) {
		add(fenced.DataFence, Object{"version": 2, "scope": "message", "id": id, "reportRef": reportID, "format": "json", "mode": "replace", "data": results[id].Rows})
	}
	add(fenced.ReportFence, Object{"version": 1, "scope": "message", "id": reportID, "mode": "commit"})
	compiled, err := fenced.Compile(&fenced.CompileRequest{Fences: fences, ReportID: reportID})
	if err != nil {
		return nil, fmt.Errorf("Forge compile: %w", err)
	}
	return &Compiled{compiled, results, p.Variant, warnings}, nil
}
func (p *Package) resolveParameters(input Object) (Object, error) {
	params := clone(input)
	if params == nil {
		params = Object{}
	}
	known := map[string]bool{}
	definitions, _ := p.Definition["parameters"].([]any)
	for _, value := range definitions {
		param, ok := value.(map[string]any)
		if !ok {
			return nil, fail("MalformedPackage", "", "parameters", "parameter must be an object")
		}
		name := str(param["name"])
		known[name] = true
		if _, ok := params[name]; !ok && param["default"] != nil {
			params[name] = param["default"]
		}
		if v, ok := params[name]; ok {
			values := []any{v}
			if param["multiple"] == true {
				var valid bool
				values, valid = v.([]any)
				if !valid {
					return nil, fail("TypeMismatch", "", name, "multiple parameter requires array")
				}
			}
			for _, v := range values {
				if !typed(v, Object{"type": param["type"], "nullable": false}) {
					return nil, fail("TypeMismatch", "", name, "parameter value has wrong type")
				}
				if options, ok := param["values"].([]any); ok {
					found := false
					for _, o := range options {
						if fmt.Sprint(o) == fmt.Sprint(v) {
							found = true
						}
					}
					if !found {
						return nil, fail("TypeMismatch", "", name, "parameter value not in allowed values")
					}
				}
			}
		}
	}
	for name := range params {
		if !known[name] {
			return nil, fail("UnknownColumn", "", name, "unknown parameter")
		}
	}
	return params, nil
}

// Native report blocks remain Forge-owned. ReportDefinition is an explicit
// compatibility adapter for the supplied examples, not a replacement grammar.
func (p *Package) forgeSource(results map[string]*Result) (Object, error) {
	if report, ok := p.Definition["report"].(map[string]any); ok {
		source := clone(report)
		blocks, ok := source["blocks"].([]any)
		if !ok {
			return nil, fail("MalformedPackage", "", "report.blocks", "expected native Forge blocks")
		}
		if err := validateBlocks(blocks, results); err != nil {
			return nil, err
		}
		return source, nil
	}
	if str(p.Definition["kind"]) != "ReportDefinition" {
		return nil, fail("MalformedPackage", "", "kind", "use report with native Forge blocks or the supplied ReportDefinition adapter")
	}
	metadata, _ := p.Definition["metadata"].(map[string]any)
	source := Object{"id": metadata["id"], "title": metadata["title"]}
	blocks := []any{}
	sections, ok := p.Definition["sections"].([]any)
	if !ok {
		return nil, fail("MalformedPackage", "", "sections", "sections must be an array")
	}
	for _, v := range sections {
		section, ok := v.(map[string]any)
		if !ok {
			return nil, fail("MalformedPackage", "", "sections", "section must be an object")
		}
		blocks = append(blocks, Object{"id": section["id"], "kind": "sectionBlock", "title": section["title"]})
		entries, ok := section["blocks"].([]any)
		if !ok {
			return nil, fail("MalformedPackage", "", "blocks", "blocks must be an array")
		}
		for _, value := range entries {
			b, ok := value.(map[string]any)
			if !ok {
				return nil, fail("MalformedPackage", "", "blocks", "block must be object")
			}
			id := str(b["id"])
			dataset := str(b["dataset"])
			r := results[dataset]
			if r == nil {
				return nil, fail("MissingDataSource", dataset, id, "block dataset is undeclared")
			}
			base := Object{"id": id, "title": b["title"], "datasetRef": dataset}
			switch str(b["type"]) {
			case "kpiGroup":
				metrics, ok := b["metrics"].([]any)
				if !ok {
					return nil, fail("MalformedPackage", dataset, id, "metrics must be an array")
				}
				for i, v := range metrics {
					m, ok := v.(map[string]any)
					if !ok {
						return nil, fail("MalformedPackage", dataset, id, "metric must be an object")
					}
					blocks = append(blocks, Object{"id": fmt.Sprintf("%s_%d", id, i), "kind": "kpiBlock", "datasetRef": dataset, "title": m["label"], "valueField": m["field"], "valueFormat": m["format"]})
				}
				continue
			case "table":
				base["kind"] = "tableBlock"
				cols := []any{}
				names, ok := b["columns"].([]any)
				if !ok {
					return nil, fail("MalformedPackage", dataset, id, "columns must be an array")
				}
				for _, name := range names {
					var col Object
					for _, c := range r.Columns {
						if c["name"] == name {
							col = c
						}
					}
					if col == nil {
						return nil, fail("UnknownColumn", dataset, str(name), "table column absent from queried dataset")
					}
					label := str(col["label"])
					if label == "" {
						label = str(name)
					}
					column := Object{"key": name, "label": label}
					if format := str(col["format"]); format != "" {
						column["format"] = format
					}
					cols = append(cols, column)
				}
				base["columns"] = cols
			case "chart":
				visual, ok := b["visual"].(map[string]any)
				if !ok {
					return nil, fail("MalformedPackage", dataset, id, "visual is required")
				}
				kind := str(visual["type"])
				if !contains([]string{"line", "bar", "horizontalBar", "area", "pie", "donut"}, kind) {
					return nil, fail("MalformedPackage", dataset, id, "unsupported chart type")
				}
				base["kind"] = "chartBlock"
				base["chartSpec"] = Object{"type": kind, "xField": visual["category"], "yFields": visual["series"]}
			default:
				return nil, fail("MalformedPackage", dataset, id, "unsupported example block type")
			}
			blocks = append(blocks, base)
		}
	}
	source["blocks"] = blocks
	if err := validateBlocks(blocks, results); err != nil {
		return nil, err
	}
	return source, nil
}
func validateBlocks(blocks []any, results map[string]*Result) error {
	for _, v := range blocks {
		b, ok := v.(map[string]any)
		if !ok {
			return fail("MalformedPackage", "", "blocks", "block must be object")
		}
		ds := str(b["datasetRef"])
		if ds == "" {
			continue
		}
		r := results[ds]
		if r == nil {
			return fail("MissingDataSource", ds, str(b["id"]), "block dataset missing")
		}
		names := map[string]bool{}
		for _, c := range r.Columns {
			names[str(c["name"])] = true
		}
		fields := []string{}
		for _, key := range []string{"valueField", "secondaryField", "timeField", "titleField", "descriptionField"} {
			if field := str(b[key]); field != "" {
				fields = append(fields, field)
			}
		}
		if cols, ok := b["columns"].([]any); ok {
			for _, v := range cols {
				if c, ok := v.(map[string]any); ok {
					fields = append(fields, str(c["key"]))
				}
			}
		}
		if chart, ok := b["chartSpec"].(map[string]any); ok {
			fields = append(fields, str(chart["xField"]))
			if ys, ok := chart["yFields"].([]any); ok {
				for _, y := range ys {
					fields = append(fields, str(y))
				}
			}
		}
		for _, field := range fields {
			if !names[field] {
				return fail("UnknownColumn", ds, field, "block references missing queried column")
			}
		}
	}
	return nil
}
func (c *Compiled) PDF() ([]byte, error) {
	model, err := reportprint.DecodeJSON(c.ReportPrint)
	if err != nil {
		return nil, err
	}
	rendered, err := forgepdf.Render(model, forgepdf.Options{ReportSpec: c.ReportSpec, CreationDate: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return nil, err
	}
	return rendered.Bytes, nil
}

func isErrorResponse(v any) bool { obj, _ := v.(map[string]any); return obj["status"] == "error" }

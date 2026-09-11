package preview

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func demo(t *testing.T, name string) *Package {
	t.Helper()
	p, e := Load(filepath.Join("..", "examples", name), "")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func query(t *testing.T, p *Package, id, raw string) *Result {
	t.Helper()
	var q Query
	if e := json.Unmarshal([]byte(raw), &q); e != nil {
		t.Fatal(e)
	}
	r, e := p.Execute(id, q)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	var d *Diagnostic
	if !errors.As(e, &d) || d.Code != "mockPreview"+want {
		t.Fatalf("got %v, want %s", e, want)
	}
}
func TestDemoCompilationAndFullExport(t *testing.T) {
	for _, name := range []string{"demo", "demo2"} {
		t.Run(name, func(t *testing.T) {
			p := demo(t, name)
			paged, e := p.Compile(nil, false)
			if e != nil {
				t.Fatal(e)
			}
			full, e := p.Compile(nil, true)
			if e != nil {
				t.Fatal(e)
			}
			id := "operationsEvidence"
			if name == "demo2" {
				id = "orderEvidence"
			}
			if len(full.Results[id].Rows) <= len(paged.Results[id].Rows) {
				t.Fatal("export did not drain browser page")
			}
			pdf, e := full.PDF()
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatal("invalid PDF")
			}
			again, e := full.PDF()
			if e != nil || !bytes.Equal(pdf, again) {
				t.Fatal("nondeterministic PDF")
			}
			if bytes.Contains(pdf, []byte("Synthetic mock preview")) {
				t.Fatal("preview label leaked into final report")
			}
		})
	}
}
func TestQueryOrderAndEnvelope(t *testing.T) {
	p := demo(t, "demo")
	r := query(t, p, "operations", `{"filter":{"and":[{"field":"region","op":"in","value":["North","Central"]},{"not":{"field":"operationsDate","op":"lt","value":"2026-08-02"}}]},"projection":{"dimensions":["region"],"measures":[{"field":"processedUnits","aggregation":"sum","alias":"total"}]},"orderBy":[{"field":"total","direction":"desc"}],"page":{"limit":1,"offset":1}}`)
	if len(r.Rows) != 1 || r.Rows[0]["region"] != "Central" || r.Rows[0]["total"] != float64(873000) || r.HasMore {
		t.Fatalf("unexpected query: %+v", r)
	}
	response := r.Response.(map[string]any)
	if response["status"] != "ok" || response["meta"] == nil {
		t.Fatal("envelope lost")
	}
	result := response["data"].([]any)[0].(map[string]any)
	if _, ok := result["rows"].([][]any); !ok {
		t.Fatal("not a matrix response")
	}
	if r.Columns[1]["format"] != "compactNumber" {
		t.Fatal("metadata lost")
	}
}
func TestQueryValidation(t *testing.T) {
	p := demo(t, "demo")
	cases := []struct{ q, code string }{
		{`{"filter":{"and":[]}}`, "UnsupportedOperator"},
		{`{"filter":{"field":"processedUnits","op":"eq","value":"100"}}`, "TypeMismatch"},
		{`{"filter":{"field":"processedUnits","op":"eq","value":null}}`, "TypeMismatch"},
		{`{"filter":{"field":"missing","op":"isNull"}}`, "UnknownColumn"},
		{`{"filter":{"field":"processedUnits","op":"contains","value":"1"}}`, "UnsupportedOperator"},
		{`{"projection":{"dimensions":["region","region"]}}`, "UnknownColumn"},
		{`{"projection":{"measures":[{"field":"operatingCost","aggregation":"count"}]}}`, "UnsafeAggregation"},
		{`{"orderBy":[{"field":"operatingCost","direction":"up"}]}`, "InvalidSort"},
		{`{"page":{"limit":0}}`, "InvalidPage"},
		{`{"page":{"limit":1,"offset":-1}}`, "InvalidPage"},
	}
	for _, tc := range cases {
		t.Run(tc.q, func(t *testing.T) {
			var q Query
			_ = json.Unmarshal([]byte(tc.q), &q)
			_, e := p.Execute("operations", q)
			code(t, e, tc.code)
		})
	}
	d := p.Extension.DataSources["operations"]
	d.Query.Filtering = false
	p.Extension.DataSources["operations"] = d
	_, e := p.Execute("operations", Query{Filter: &Predicate{Field: "region", Op: "eq", Value: "North"}})
	code(t, e, "UnsupportedCapability")
}
func TestRecordToTabularAndIsolation(t *testing.T) {
	p := demo(t, "demo2")
	s := p.Sources["processTiming"]
	s.ResultContract = Contract{Shape: "tabular", ResultName: "timing", ResultsPath: "data"}
	p.Sources["processTiming"] = s
	r := query(t, p, "processTiming", `{}`)
	obj := r.Response.(map[string]any)
	if obj["data"] == nil {
		t.Fatal("record fixture did not use declared tabular output")
	}
	r.Rows[0]["planningDays"] = 999.0
	r.Columns[0]["name"] = "changed"
	again := query(t, p, "processTiming", `{}`)
	if again.Rows[0]["planningDays"] == 999.0 || again.Columns[0]["name"] == "changed" {
		t.Fatal("query mutated fixture")
	}
}
func TestParameters(t *testing.T) {
	p := demo(t, "demo2")
	c, e := p.Compile(Object{"region": []any{"North"}, "scenario": "Actual"}, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range c.Results["orderEvidence"].Rows {
		if r["region"] != "North" || r["scenario"] != "Actual" {
			t.Fatal(r)
		}
	}
	_, e = p.Compile(Object{"scenario": "invalid"}, false)
	code(t, e, "TypeMismatch")
	_, e = p.Compile(Object{"identity": "admin"}, false)
	code(t, e, "UnknownColumn")
}
func TestConcurrentDeterminism(t *testing.T) {
	p := demo(t, "demo")
	expected := query(t, p, "operations", `{}`)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := p.Execute("operations", Query{})
			if e != nil || !reflect.DeepEqual(r, expected) {
				t.Errorf("non-deterministic query: %v", e)
			}
		}()
	}
	wg.Wait()
}
func copyDemo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS(filepath.Join("..", "examples", "demo"))); e != nil {
		t.Fatal(e)
	}
	return root
}
func TestPackageFailures(t *testing.T) {
	cases := []struct {
		name, code string
		mutate     func(string)
	}{
		{"missing", "MissingDataSource", func(root string) { _ = os.Remove(filepath.Join(root, "ds", "operations.json")) }},
		{"version", "MalformedPackage", func(root string) {
			p := filepath.Join(root, "report.yaml")
			b, _ := os.ReadFile(p)
			_ = os.WriteFile(p, bytes.Replace(b, []byte("version: 1"), []byte("version: 2"), 1), 0644)
		}},
		{"duplicate YAML", "MalformedPackage", func(root string) {
			p := filepath.Join(root, "report.yaml")
			b, _ := os.ReadFile(p)
			_ = os.WriteFile(p, append(b, []byte("\nkind: Duplicate\n")...), 0644)
		}},
		{"bad JSON", "MalformedTabularResult", func(root string) {
			_ = os.WriteFile(filepath.Join(root, "ds", "operations.json"), []byte(`{"status":"ok",}`), 0644)
		}},
		{"variant changes report", "MalformedPackage", func(root string) {
			_ = os.MkdirAll(filepath.Join(root, "variants", "empty"), 0755)
			_ = os.WriteFile(filepath.Join(root, "variants", "empty", "report.yaml"), []byte("kind: evil"), 0644)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyDemo(t)
			tc.mutate(root)
			variant := ""
			if strings.HasPrefix(tc.name, "variant") {
				variant = "empty"
			}
			_, e := Load(root, variant)
			code(t, e, tc.code)
		})
	}
}
func TestFixtureValidation(t *testing.T) {
	p := demo(t, "demo")
	d := p.Extension.DataSources["operations"]
	s := p.Sources["operations"]
	base := p.fixtures["operations"].raw
	for _, tc := range []struct {
		name, code string
		mutate     func(Object)
	}{
		{"width", "RowWidthMismatch", func(r Object) { r["rows"].([]any)[0] = []any{"short"} }},
		{"type", "TypeMismatch", func(r Object) { r["rows"].([]any)[0].([]any)[2] = "420000" }},
		{"null", "TypeMismatch", func(r Object) { r["rows"].([]any)[0].([]any)[0] = nil }},
		{"precision", "TypeMismatch", func(r Object) { r["rows"].([]any)[0].([]any)[2] = float64(9007199254740992) }},
		{"duplicate column", "MalformedTabularResult", func(r Object) { r["columns"].([]any)[1].(map[string]any)["name"] = "operationsDate" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := clone(base).(map[string]any)
			tc.mutate(raw["data"].([]any)[0].(map[string]any))
			b, _ := json.Marshal(raw)
			_, e := decodeFixture("operations", b, d, s)
			code(t, e, tc.code)
		})
	}
}
func TestSymlinkEscape(t *testing.T) {
	root := copyDemo(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	_ = os.WriteFile(outside, []byte(`{}`), 0644)
	path := filepath.Join(root, "ds", "operations.json")
	_ = os.Remove(path)
	if e := os.Symlink(outside, path); e != nil {
		t.Fatal(e)
	}
	_, e := readLocal(root, "ds/operations.json", 100)
	code(t, e, "PathEscape")
}

func TestVariants(t *testing.T) {
	for _, name := range []string{"demo", "demo2"} {
		for _, variant := range []string{"empty", "error", "partial", "nulls", "large"} {
			t.Run(name+"/"+variant, func(t *testing.T) {
				p, e := Load(filepath.Join("..", "examples", name), variant)
				if e != nil {
					t.Fatal(e)
				}
				c, e := p.Compile(nil, false)
				if variant == "error" {
					code(t, e, "DataSourceError")
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				if _, e = c.PDF(); e != nil {
					t.Fatal(e)
				}
				if variant == "partial" {
					_, e = p.Compile(nil, true)
					code(t, e, "IncompleteData")
				}
			})
		}
	}
}
func TestHostLimitsAndAliases(t *testing.T) {
	_, e := LoadWithLimits(filepath.Join("..", "examples", "demo"), "", Limits{Rows: 1})
	code(t, e, "LimitExceeded")
	p := demo(t, "demo")
	r := query(t, p, "operations", `{"projection":{"dimensions":["region",{"field":"region","alias":"second"}],"measures":[{"field":"processedUnits","aggregation":"sum"}]}}`)
	if r.Rows[0]["region"] != r.Rows[0]["second"] {
		t.Fatal("alias not preserved")
	}
	p.limits.PredicateLeaves = 1
	_, e = p.Execute("operations", Query{Filter: &Predicate{And: []Predicate{{Field: "region", Op: "eq", Value: "North"}, {Field: "region", Op: "neq", Value: "South"}}}})
	code(t, e, "LimitExceeded")
}
func TestPredicatesAndAggregations(t *testing.T) {
	schema := map[string]Object{"n": {"type": "number", "nullable": true}, "s": {"type": "string", "nullable": false}, "date": {"type": "date", "nullable": false}, "ts": {"type": "timestamp", "nullable": false}}
	row := Object{"n": 3.0, "s": "Alpha beta", "date": "2026-08-15", "ts": "2026-08-01T01:00:00+01:00"}
	cases := []struct {
		field, op string
		value     any
		want      bool
	}{{"n", "eq", 3.0, true}, {"n", "neq", 4.0, true}, {"n", "lt", 3.0, false}, {"n", "lte", 3.0, true}, {"n", "gt", 2.0, true}, {"n", "gte", 3.0, true}, {"n", "between", []any{3.0, 4.0}, true}, {"n", "in", []any{3.0}, true}, {"n", "notIn", []any{4.0}, true}, {"n", "isNull", nil, false}, {"n", "isNotNull", nil, true}, {"s", "contains", "beta", true}, {"s", "startsWith", "Alpha", true}, {"s", "endsWith", "beta", true}, {"s", "contains", "alpha", false}, {"date", "between", []any{"2026-08-01", "2026-08-31"}, true}, {"ts", "gte", "2026-08-01T00:00:00Z", true}}
	for _, tc := range cases {
		t.Run(tc.field+tc.op, func(t *testing.T) {
			leaves := 0
			fn, e := compilePredicate("test", Predicate{Field: tc.field, Op: tc.op, Value: tc.value}, schema, Declaration{}, &leaves, 0)
			if e != nil || fn(row) != tc.want {
				t.Fatalf("%v", e)
			}
		})
	}
	for op, want := range map[string]any{"sum": 8.0, "min": 2.0, "max": 3.0, "count": 3.0, "countDistinct": 2.0, "average": 8.0 / 3} {
		got, e := aggregate("test", Field{Field: "n", Aggregation: op}, []Object{{"n": 2.0}, {"n": 3.0}, {"n": 3.0}, {"n": nil}}, schema["n"])
		if e != nil || got != want {
			t.Fatalf("%s: got %v want %v (%v)", op, got, want, e)
		}
	}
}
func TestNativeForgeBlocks(t *testing.T) {
	p := demo(t, "demo")
	p.Definition["report"] = Object{"id": "native", "title": "Native report", "blocks": []any{Object{"id": "table", "kind": "tableBlock", "datasetRef": "operationsEvidence", "columns": []any{Object{"key": "region", "label": "Region"}, Object{"key": "processedUnits", "label": "Processed Units"}}}}}
	c, e := p.Compile(nil, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.PDF(); e != nil {
		t.Fatal(e)
	}
}

func TestTabularExportsAndHTML(t *testing.T) {
	p := demo(t, "demo")
	c, e := p.Compile(nil, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, format := range []string{"csv", "xlsx", "html"} {
		b, e := c.Export(format, "operationsTable")
		if e != nil {
			t.Fatal(e)
		}
		switch format {
		case "csv":
			if bytes.Count(b, []byte("2026-08-")) != 12 {
				t.Fatal("CSV omitted full rows")
			}
		case "xlsx":
			if !bytes.HasPrefix(b, []byte("PK")) {
				t.Fatal("not XLSX")
			}
		case "html":
			if !bytes.Contains(b, []byte("data:image/svg+xml;base64,")) || !bytes.Contains(b, []byte("3.29M")) {
				t.Fatal("HTML missing compiled chart or KPI")
			}
		}
	}
}
func TestNullableStableSortAndPageCap(t *testing.T) {
	p, e := Load(filepath.Join("..", "examples", "demo2"), "nulls")
	if e != nil {
		t.Fatal(e)
	}
	r := query(t, p, "processTiming", `{"orderBy":[{"field":"planningDays","direction":"desc"}],"page":{"limit":5000}}`)
	if r.Rows[len(r.Rows)-1]["planningDays"] != nil {
		t.Fatal("descending nulls must be last")
	}
	r = query(t, p, "processTiming", `{"orderBy":[{"field":"planningDays","direction":"asc","nulls":"first"}]}`)
	if r.Rows[0]["planningDays"] != nil {
		t.Fatal("explicit null ordering ignored")
	}
	p.Extension.MaxPageSize = 1
	r = query(t, p, "processTiming", `{"page":{"limit":5000}}`)
	if len(r.Rows) != 1 || !r.HasMore {
		t.Fatal("page cap ignored")
	}
}
func TestOptionalDatasourceFailure(t *testing.T) {
	p, e := Load(filepath.Join("..", "examples", "demo"), "error")
	if e != nil {
		t.Fatal(e)
	}
	d := p.Extension.DataSources["operations"]
	d.Optional = true
	p.Extension.DataSources["operations"] = d
	c, e := p.Compile(nil, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Results["targets"].Rows) == 0 || len(c.Warnings) == 0 {
		t.Fatal("optional failure did not preserve unrelated datasource and diagnostic")
	}
}
func TestUnknownDatasetQueryFieldRejected(t *testing.T) {
	var q Query
	if e := json.Unmarshal([]byte(`{"projecton":{}}`), &q); e == nil {
		t.Fatal("unknown query operation ignored")
	}
}

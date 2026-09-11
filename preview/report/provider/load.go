package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/viant/agently/preview/datasource"
	"gopkg.in/yaml.v3"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func readLocal(root, relative string, limit int64) ([]byte, error) {
	if filepath.IsAbs(relative) {
		return nil, fail("PathEscape", "", "", relative)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fail("PathEscape", "", "", relative)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fail("MalformedPackage", "", "", "expected regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, fail("LimitExceeded", "", "", relative)
	}
	return b, err
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Load(location, variant string) (*Package, error) {
	return LoadWithLimits(location, variant, Limits{})
}
func LoadWithLimits(location, variant string, limits Limits) (*Package, error) {
	limits, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(location)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	if err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, e := filepath.EvalSymlinks(path)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(root, resolved)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fail("PathEscape", "", "", path)
			}
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".js", ".mjs", ".cjs", ".py", ".sh", ".bash", ".zsh", ".go", ".exe":
			return fail("MalformedPackage", "", "", "executable scripts are prohibited: "+path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if info, e := os.Stat(filepath.Join(root, "ds")); e != nil || !info.IsDir() {
		return nil, fail("MalformedPackage", "", "ds", "base ds directory is required")
	}
	b, err := readLocal(root, "report.yaml", limits.ReportBytes)
	if err != nil {
		return nil, localDiagnostic(err, "MalformedPackage", "")
	}
	var node yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(b))
	if err = dec.Decode(&node); err != nil {
		return nil, fail("MalformedPackage", "", "", err.Error())
	}
	var extra yaml.Node
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, fail("MalformedPackage", "", "", "expected one YAML document")
	}
	var check func(*yaml.Node) error
	check = func(n *yaml.Node) error {
		if n.Tag == "!!timestamp" {
			n.Tag = "!!str"
		}
		if n.Kind == yaml.AliasNode || (n.Tag != "" && !strings.HasPrefix(n.Tag, "!!")) {
			return fail("MalformedPackage", "", "", "YAML aliases and custom tags are unsupported")
		}
		for _, c := range n.Content {
			if err := check(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err = check(&node); err != nil {
		return nil, err
	}
	var definition Object
	if err = node.Decode(&definition); err != nil {
		return nil, fail("MalformedPackage", "", "", err.Error())
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Endpoints map[string]datasource.Endpoint `json:"endpoints"`
		Extension Extension                      `json:"x-mock-preview"`
		Sources   map[string]Source              `json:"dataSources"`
		Datasets  map[string]Dataset             `json:"datasets"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fail("MalformedPackage", "", "", err.Error())
	}
	ext := doc.Extension
	if ext.Version != 1 {
		return nil, fail("MalformedPackage", "", "version", "unsupported extension version")
	}
	if len(ext.DataSources) == 0 {
		return nil, fail("MissingDataSource", "", "", "no datasources declared")
	}
	if ext.DataRoot == "" {
		ext.DataRoot = "ds"
	}
	if ext.MaxPageSize > 1000 {
		return nil, fail("InvalidPage", "", "maxPageSize", "maximum page size is 1000")
	}
	if ext.MaxPageSize == 0 {
		ext.MaxPageSize = limits.PageSize
	}
	ext.MaxPageSize = min(ext.MaxPageSize, limits.PageSize)
	if ext.MaxPageSize < 1 || ext.MaxPageSize > 1000 {
		return nil, fail("InvalidPage", "", "maxPageSize", "must be 1..1000")
	}
	if variant == "" {
		variant = ext.DefaultVariant
	}
	if variant == "" {
		variant = "default"
	}
	if !safeID.MatchString(variant) {
		return nil, fail("PathEscape", "", "variant", "unsafe variant")
	}
	if variant != "default" {
		if _, err := os.Stat(filepath.Join(root, "variants", variant)); err != nil {
			return nil, fail("MalformedPackage", "", "variant", "variant does not exist")
		}
		if _, err := os.Lstat(filepath.Join(root, "variants", variant, "report.yaml")); err == nil {
			return nil, fail("MalformedPackage", "", "variant", "variant cannot change report.yaml")
		}
	}
	p := &Package{Endpoints: doc.Endpoints, Definition: definition, Extension: ext, Sources: doc.Sources, Datasets: doc.Datasets, Variant: variant, fixtures: map[string]*fixture{}, definitionHash: hash(raw), limits: limits, queries: make(chan struct{}, limits.ConcurrentQueries)}
	for _, id := range sortedKeys(ext.DataSources) {
		d := ext.DataSources[id]
		if !safeID.MatchString(id) {
			return nil, fail("PathEscape", id, "", "unsafe datasource ID")
		}
		if d.File == "" {
			d.File = id + ".json"
		}
		if d.File != id+".json" {
			return nil, fail("MalformedPackage", id, "file", "filename must match datasource ID")
		}
		body, err := readLocal(root, filepath.Join(ext.DataRoot, d.File), limits.FixtureBytes)
		if err != nil {
			return nil, localDiagnostic(err, "MissingDataSource", id)
		}
		if variant != "default" {
			vpath := filepath.Join("variants", variant, "ds", d.File)
			if _, err := os.Lstat(filepath.Join(root, vpath)); err == nil {
				overlay, e := readLocal(root, vpath, limits.FixtureBytes)
				if e != nil {
					return nil, e
				}
				if bytes.Equal(body, overlay) {
					p.Warnings = append(p.Warnings, "variant fixture identical to base: "+id)
				}
				body = overlay
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		if len(body) > 10<<20 {
			p.Warnings = append(p.Warnings, "large fixture: "+id)
		}
		source, ok := doc.Sources[id]
		if !ok {
			return nil, fail("MissingDataSource", id, "", "missing report datasource declaration")
		}
		if d.InputContract.Shape == "" {
			d.InputContract = source.ResultContract
		}
		if d.ResultName == "" {
			d.ResultName = source.ResultContract.ResultName
		}
		if d.ResultName == "" {
			d.ResultName = id
		}
		ext.DataSources[id] = d
		if variant != "default" {
			base, e := readLocal(root, filepath.Join(ext.DataRoot, d.File), limits.FixtureBytes)
			if e != nil {
				return nil, localDiagnostic(e, "MissingDataSource", id)
			}
			if _, e = decodeFixture(id, base, d, source); e != nil {
				return nil, e
			}
		}
		for _, contract := range []Contract{d.InputContract, source.ResultContract} {
			if contract.Shape != "records" && contract.Shape != "tabular" {
				return nil, fail("MalformedPackage", id, "contract.shape", "expected records or tabular")
			}
			if contract.Shape == "records" && contract.RowPath == "" {
				return nil, fail("MalformedPackage", id, "rowPath", "records require an explicit rowPath")
			}
		}
		if d.Query.Pagination != "" && d.Query.Pagination != "offset" {
			return nil, fail("UnsupportedCapability", id, "pagination", "only offset pagination is supported")
		}
		f, err := decodeFixture(id, body, d, source)
		if err != nil {
			return nil, err
		}
		if len(f.rows) > limits.Rows || len(f.columns) > limits.Columns {
			return nil, fail("LimitExceeded", id, "", "host row or column limit exceeded")
		}
		// Validate every named result, including unselected results in a live envelope.
		if d.InputContract.Shape == "tabular" && !f.errorResponse {
			path := d.InputContract.ResultsPath
			if path == "" {
				path = "data"
			}
			results, _ := at(f.raw, path).([]any)
			for _, value := range results {
				result := value.(map[string]any)
				if result["name"] == d.ResultName {
					continue
				}
				other := d
				other.ResultName = str(result["name"])
				other.Dimensions = nil
				other.Measures = nil
				if _, e := decodeFixture(id, body, other, Source{}); e != nil {
					return nil, e
				}
			}
		}
		p.fixtures[id] = f
	}
	p.Extension = ext
	parameters := map[string]bool{}
	if values, ok := definition["parameters"].([]any); ok {
		for _, v := range values {
			param, ok := v.(map[string]any)
			if !ok {
				return nil, fail("MalformedPackage", "", "parameters", "parameter must be an object")
			}
			name := str(param["name"])
			if !safeID.MatchString(name) || parameters[name] || !validType(str(param["type"])) {
				return nil, fail("MalformedPackage", "", name, "invalid or duplicate parameter")
			}
			parameters[name] = true
		}
	}
	for _, id := range sortedKeys(p.Sources) {
		if p.fixtures[id] == nil {
			return nil, fail("MissingDataSource", id, "", "report datasource has no fixture declaration")
		}
		fields := map[string]bool{}
		for _, c := range p.fixtures[id].columns {
			fields[str(c["name"])] = true
		}
		for _, binding := range p.Sources[id].ParameterBindings {
			if !parameters[binding.Parameter] || !fields[binding.Field] {
				return nil, fail("UnknownColumn", id, binding.Field, "parameter binding references unknown parameter or field")
			}
		}
	}
	for id := range doc.Sources {
		if _, ok := ext.DataSources[id]; !ok {
			return nil, fail("MissingDataSource", id, "", "report datasource has no fixture declaration")
		}
	}
	if ext.PrimaryDataSource != "" {
		if _, ok := ext.DataSources[ext.PrimaryDataSource]; !ok {
			return nil, fail("MissingDataSource", ext.PrimaryDataSource, "", "primary datasource is undeclared")
		}
	}
	for _, id := range sortedKeys(p.Datasets) {
		ds := p.Datasets[id]
		if !safeID.MatchString(id) {
			return nil, fail("MalformedPackage", ds.DataSource, id, "unsafe dataset ID")
		}
		if _, ok := p.fixtures[ds.DataSource]; !ok {
			return nil, fail("MissingDataSource", ds.DataSource, id, "dataset references unknown datasource")
		}
		if _, err := p.Execute(ds.DataSource, ds.Query); err != nil {
			return nil, err
		}
	}
	entries, _ := os.ReadDir(filepath.Join(root, ext.DataRoot))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			if _, ok := ext.DataSources[strings.TrimSuffix(e.Name(), ".json")]; !ok {
				p.Warnings = append(p.Warnings, "unused datasource fixture: "+e.Name())
			}
		}
	}
	results := map[string]*Result{}
	for _, id := range sortedKeys(p.Datasets) {
		ds := p.Datasets[id]
		r, e := p.Execute(ds.DataSource, ds.Query)
		if e != nil {
			return nil, e
		}
		results[id] = r
	}
	if _, err := p.forgeSource(results); err != nil {
		return nil, err
	}
	p.Warnings = append(p.Warnings, p.unusedColumnWarnings()...)
	return p, nil
}
func sortedKeys[T any](m map[string]T) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func at(v any, path string) any {
	if path == "$" || path == "" {
		return v
	}
	for _, key := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[key]
	}
	return v
}
func decodeFixture(id string, b []byte, d Declaration, s Source) (*fixture, error) {
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fail("MalformedTabularResult", id, "", err.Error())
	}
	f := &fixture{raw: raw, hash: hash(b), rows: []Object{}}
	if obj, ok := raw.(map[string]any); ok {
		status, ok := obj["status"].(string)
		if !ok {
			return nil, fail("MalformedTabularResult", id, "status", "status must be a string")
		}
		if status != "ok" {
			if status != "error" || str(obj["message"]) == "" {
				return nil, fail("MalformedTabularResult", id, "status", "expected ok or error with message")
			}
			f.errorResponse = true
			// Validate the declared schema even for a stable error fixture.
			raw = Object{"status": "ok"}
			if d.InputContract.Shape == "tabular" {
				path := d.InputContract.ResultsPath
				if path == "" {
					path = "data"
				}
				setAt(raw.(map[string]any), path, []any{Object{"name": d.ResultName, "columns": toAny(s.Columns), "rows": []any{}, "hasMore": false}})
			} else {
				if d.InputContract.RowPath == "$" {
					raw = []any{}
				} else {
					setAt(raw.(map[string]any), d.InputContract.RowPath, []any{})
					if d.InputContract.HasMorePath != "" {
						setAt(raw.(map[string]any), d.InputContract.HasMorePath, false)
					}
				}
			}
		}
	}
	var rows []any
	switch d.InputContract.Shape {
	case "tabular":
		path := d.InputContract.ResultsPath
		if path == "" {
			path = "data"
		}
		results, ok := at(raw, path).([]any)
		if !ok {
			return nil, fail("MalformedTabularResult", id, "", "results must be an array")
		}
		var selected Object
		names := map[string]bool{}
		for _, v := range results {
			r, ok := v.(map[string]any)
			if !ok {
				return nil, fail("MalformedTabularResult", id, "", "result must be an object")
			}
			name := str(r["name"])
			if name == "" || names[name] {
				return nil, fail("MalformedTabularResult", id, "", "empty or duplicate result name")
			}
			names[name] = true
			if name == d.ResultName {
				selected = r
			}
		}
		if selected == nil {
			return nil, fail("MalformedTabularResult", id, "resultName", "selected result missing")
		}
		cols, ok := selected["columns"].([]any)
		if !ok {
			return nil, fail("MalformedTabularResult", id, "columns", "columns must be an array")
		}
		for _, v := range cols {
			c, ok := v.(map[string]any)
			if !ok {
				return nil, fail("MalformedTabularResult", id, "columns", "column must be an object")
			}
			f.columns = append(f.columns, c)
		}
		rows, ok = selected["rows"].([]any)
		if !ok {
			return nil, fail("MalformedTabularResult", id, "rows", "rows must be an array")
		}
		more, ok := selected["hasMore"].(bool)
		if !ok {
			return nil, fail("MalformedTabularResult", id, "hasMore", "hasMore must be boolean")
		}
		f.hasMore = more
	case "records":
		f.columns = s.Columns
		var ok bool
		rows, ok = at(raw, d.InputContract.RowPath).([]any)
		if !ok {
			return nil, fail("MalformedTabularResult", id, "rowPath", "expected array")
		}
		if d.InputContract.HasMorePath != "" {
			more, ok := at(raw, d.InputContract.HasMorePath).(bool)
			if !ok {
				return nil, fail("MalformedTabularResult", id, "hasMore", "expected boolean")
			}
			f.hasMore = more
		}
	default:
		return nil, fail("MalformedPackage", id, "inputContract.shape", "expected tabular or records")
	}
	if len(rows) > 100000 || len(f.columns) > 500 {
		return nil, fail("LimitExceeded", id, "", "too many rows or columns")
	}
	schema := map[string]Object{}
	for _, c := range f.columns {
		name := str(c["name"])
		if name == "" || schema[name] != nil {
			return nil, fail("MalformedTabularResult", id, "columns", "empty or duplicate column")
		}
		if _, ok := c["nullable"].(bool); !ok {
			return nil, fail("MalformedTabularResult", id, name, "nullable must be boolean")
		}
		if !validType(str(c["type"])) {
			return nil, fail("TypeMismatch", id, name, "unsupported column type")
		}
		schema[name] = c
	}
	for _, c := range s.Columns {
		actual := schema[str(c["name"])]
		if actual == nil {
			return nil, fail("UnknownColumn", id, str(c["name"]), "declared column missing from response")
		}
		for _, key := range []string{"label", "description", "provenance"} {
			if actual[key] == nil && c[key] != nil {
				actual[key] = clone(c[key])
			}
		}
		for _, key := range []string{"type", "role", "format", "nullable"} {
			if fmt.Sprint(actual[key]) != fmt.Sprint(c[key]) {
				return nil, fail("TypeMismatch", id, str(c["name"]), "fixture metadata differs from declaration: "+key)
			}
		}
	}
	for _, name := range d.Dimensions {
		if schema[name] == nil {
			return nil, fail("UnknownColumn", id, name, "dimension missing")
		}
	}
	for name, m := range d.Measures {
		if schema[name] == nil {
			return nil, fail("UnknownColumn", id, name, "measure missing")
		}
		if m.Aggregation == "sum" && (schema[name]["format"] == "percent" || schema[name]["format"] == "percentFraction" || contains([]string{"ratio", "rate", "lift", "roas", "share", "cost"}, strings.ToLower(str(schema[name]["semanticType"])))) {
			return nil, fail("UnsafeAggregation", id, name, "non-additive measure cannot be summed")
		}
		if (m.Aggregation == "sum" || m.Aggregation == "average") && schema[name]["type"] != "integer" && schema[name]["type"] != "number" {
			return nil, fail("UnsafeAggregation", id, name, "numeric aggregation requires numeric column")
		}
		switch m.Aggregation {
		case "none", "sum", "min", "max", "count", "countDistinct":
		case "average":
			if !m.RowLevel {
				return nil, fail("UnsafeAggregation", id, name, "average requires rowLevel")
			}
		default:
			return nil, fail("UnsafeAggregation", id, name, "unknown aggregation")
		}
	}
	for i, v := range rows {
		row := Object{}
		if d.InputContract.Shape == "tabular" {
			matrix, ok := v.([]any)
			if !ok || len(matrix) != len(f.columns) {
				return nil, fail("RowWidthMismatch", id, fmt.Sprint(i), "row width differs from columns")
			}
			for j, c := range f.columns {
				row[str(c["name"])] = matrix[j]
			}
		} else {
			record, ok := v.(map[string]any)
			if !ok {
				return nil, fail("TypeMismatch", id, fmt.Sprint(i), "record must be an object")
			}
			for _, c := range f.columns {
				name := str(c["name"])
				row[name] = record[name]
			}
		}
		for _, c := range f.columns {
			name := str(c["name"])
			if !typed(row[name], c) {
				return nil, fail("TypeMismatch", id, fmt.Sprintf("rows.%d.%s", i, name), "value violates type or nullability")
			}
		}
		f.rows = append(f.rows, row)
	}
	return f, nil
}
func validType(t string) bool {
	switch t {
	case "string", "date", "timestamp", "integer", "number", "boolean", "array", "object", "decimal":
		return true
	}
	return false
}
func typed(v any, c Object) bool {
	if v == nil {
		return c["nullable"] == true
	}
	switch str(c["type"]) {
	case "string":
		_, ok := v.(string)
		return ok
	case "decimal":
		s, ok := v.(string)
		return ok && regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`).MatchString(s)
	case "date":
		s, ok := v.(string)
		if !ok {
			return false
		}
		t, e := time.Parse("2006-01-02", s)
		return e == nil && t.Format("2006-01-02") == s
	case "timestamp":
		s, ok := v.(string)
		if !ok {
			return false
		}
		_, e := time.Parse(time.RFC3339Nano, s)
		return e == nil
	case "integer", "number":
		n, ok := v.(float64)
		return ok && !math.IsInf(n, 0) && !math.IsNaN(n) && (str(c["type"]) != "integer" || (math.Trunc(n) == n && math.Abs(n) <= 9007199254740991))
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return false
}

func localDiagnostic(err error, code, id string) error {
	var d *Diagnostic
	if errors.As(err, &d) {
		return err
	}
	return fail(code, id, "", err.Error())
}
func toAny(columns []Object) []any {
	result := make([]any, len(columns))
	for i, c := range columns {
		result[i] = c
	}
	return result
}

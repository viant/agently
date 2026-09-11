package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// Query is the renderer-neutral basic query accepted by every fixture tool.
// Aggregation is intentionally left to a host-supplied Transform.
type Query struct {
	ResultName string      `json:"resultName,omitempty"`
	Projection *Projection `json:"projection,omitempty"`
	Filter     *Predicate  `json:"filter,omitempty"`
	OrderBy    []Order     `json:"orderBy,omitempty"`
	Page       *Page       `json:"page,omitempty"`
}
type Projection struct {
	Fields     []Field `json:"fields,omitempty"`
	Dimensions []Field `json:"dimensions,omitempty"`
	Measures   []Field `json:"measures,omitempty"`
}
type Field struct {
	Field       string `json:"field"`
	Alias       string `json:"alias,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
}

func (f *Field) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &f.Field)
	}
	type plain Field
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode((*plain)(f))
}
func (f Field) name() string {
	if f.Alias != "" {
		return f.Alias
	}
	return f.Field
}

type Predicate struct {
	And   []Predicate `json:"and,omitempty"`
	Or    []Predicate `json:"or,omitempty"`
	Not   *Predicate  `json:"not,omitempty"`
	Field string      `json:"field,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value any         `json:"value,omitempty"`
}
type Order struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
	Nulls     string `json:"nulls,omitempty"`
}
type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// ApplyQuery preserves the fixture envelope and column metadata. Supported input
// shapes are a bare record array, {rows:[...]}, and {data:[{name,columns,rows}]}.
func ApplyQuery(body json.RawMessage, query Query) (any, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	obj, _ := raw.(map[string]any)
	if obj != nil && obj["status"] == "error" {
		return raw, nil
	}
	var container map[string]any
	var values []any
	tabular := false
	if array, ok := raw.([]any); ok {
		values = array
	} else if rows, ok := obj["rows"].([]any); ok {
		values = rows
		container = obj
	} else if results, ok := obj["data"].([]any); ok {
		for _, v := range results {
			r, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("tabular result must be an object")
			}
			if query.ResultName == "" && len(results) == 1 || r["name"] == query.ResultName {
				if container != nil {
					return nil, fmt.Errorf("duplicate selected result")
				}
				container = r
			}
		}
		if container == nil {
			return nil, fmt.Errorf("query.resultName must select one named result")
		}
		var ok bool
		values, ok = container["rows"].([]any)
		if !ok {
			return nil, fmt.Errorf("result rows must be an array")
		}
		tabular = true
	} else {
		return nil, fmt.Errorf("query requires records or a named tabular response")
	}
	if len(values) > 100000 {
		return nil, fmt.Errorf("row limit exceeded")
	}
	columns := []map[string]any{}
	schema := map[string]map[string]any{}
	rows := []map[string]any{}
	if tabular {
		cols, ok := container["columns"].([]any)
		if !ok {
			return nil, fmt.Errorf("columns must be an array")
		}
		for _, v := range cols {
			c, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("column must be an object")
			}
			name, _ := c["name"].(string)
			if name == "" || schema[name] != nil {
				return nil, fmt.Errorf("empty or duplicate column name")
			}
			schema[name] = c
			columns = append(columns, c)
		}
	}
	for _, v := range values {
		if tabular {
			cells, ok := v.([]any)
			if !ok || len(cells) != len(columns) {
				return nil, fmt.Errorf("row width differs from columns")
			}
			row := map[string]any{}
			for i, c := range columns {
				row[c["name"].(string)] = cells[i]
			}
			rows = append(rows, row)
		} else {
			row, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("record must be an object")
			}
			rows = append(rows, row)
			for name, value := range row {
				c := schema[name]
				if c == nil {
					c = map[string]any{"name": name}
					schema[name] = c
				}
				kind := valueType(value)
				if kind != "" {
					if old, _ := c["type"].(string); old != "" && old != kind {
						c["type"] = "mixed"
					} else {
						c["type"] = kind
					}
				}
			}
		}
	}
	if !tabular {
		names := []string{}
		for name := range schema {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			columns = append(columns, schema[name])
		}
	}
	if len(columns) > 500 {
		return nil, fmt.Errorf("column limit exceeded")
	}
	// An empty record fixture has no discoverable schema. Projection provides names;
	// report hosts can supply authoritative schemas through their Transform.
	if len(rows) == 0 && !tabular && query.Projection != nil {
		for _, f := range projectionFields(query.Projection) {
			schema[f.Field] = map[string]any{"name": f.Field}
		}
	}
	var test func(map[string]any) bool
	if query.Filter != nil {
		leaves := 0
		var err error
		test, err = predicate(*query.Filter, schema, &leaves, 0)
		if err != nil {
			return nil, err
		}
	}
	selected := []map[string]any{}
	for _, r := range rows {
		if test == nil || test(r) {
			selected = append(selected, r)
		}
	}
	fields := []Field{}
	if query.Projection != nil {
		if len(query.Projection.Fields) > 0 && (len(query.Projection.Dimensions) > 0 || len(query.Projection.Measures) > 0) {
			return nil, fmt.Errorf("use projection.fields or dimensions/measures, not both")
		}
		fields = projectionFields(query.Projection)
		if len(fields) == 0 {
			return nil, fmt.Errorf("empty projection")
		}
	} else {
		for _, c := range columns {
			fields = append(fields, Field{Field: c["name"].(string)})
		}
	}
	if len(fields) > 500 {
		return nil, fmt.Errorf("projection column limit exceeded")
	}
	outputColumns := []map[string]any{}
	names := map[string]bool{}
	for _, f := range fields {
		c := schema[f.Field]
		if c == nil {
			return nil, fmt.Errorf("unknown projection field %q", f.Field)
		}
		if f.Aggregation != "" && f.Aggregation != "none" {
			return nil, fmt.Errorf("basic fixture tools do not support aggregation; use a query transform")
		}
		if names[f.name()] {
			return nil, fmt.Errorf("duplicate output field %q", f.name())
		}
		names[f.name()] = true
		copy := map[string]any{}
		for k, v := range c {
			copy[k] = v
		}
		copy["name"] = f.name()
		outputColumns = append(outputColumns, copy)
	}
	output := []map[string]any{}
	for _, r := range selected {
		row := map[string]any{}
		for _, f := range fields {
			row[f.name()] = r[f.Field]
		}
		output = append(output, row)
	}
	for _, o := range query.OrderBy {
		if !names[o.Field] {
			return nil, fmt.Errorf("sort field %q must be projected", o.Field)
		}
		if o.Direction != "asc" && o.Direction != "desc" {
			return nil, fmt.Errorf("sort direction must be asc or desc")
		}
		if o.Nulls != "" && o.Nulls != "first" && o.Nulls != "last" {
			return nil, fmt.Errorf("nulls must be first or last")
		}
		var sortType string
		for _, r := range output {
			if kind := valueType(r[o.Field]); kind != "" {
				if sortType != "" && sortType != kind {
					return nil, fmt.Errorf("sort field has mixed types")
				}
				sortType = kind
			}
			if r[o.Field] != nil && !scalar(r[o.Field]) {
				return nil, fmt.Errorf("sort field must be scalar")
			}
		}
	}
	sort.SliceStable(output, func(i, j int) bool {
		for _, o := range query.OrderBy {
			a, b := output[i][o.Field], output[j][o.Field]
			if a == nil || b == nil {
				if a == nil && b == nil {
					continue
				}
				if o.Nulls == "first" {
					return a == nil
				}
				return b == nil
			}
			cmp := compare(a, b)
			if cmp != 0 {
				if o.Direction == "desc" {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		return false
	})
	limit, offset := 1000, 0
	if query.Page != nil {
		if query.Page.Limit < 1 || query.Page.Offset < 0 {
			return nil, fmt.Errorf("positive limit and nonnegative offset required")
		}
		limit = min(query.Page.Limit, 1000)
		offset = query.Page.Offset
	}
	total := len(output)
	start := min(offset, total)
	end := start + min(limit, total-start)
	output = output[start:end]
	more := end < total
	if container != nil {
		if upstream, _ := container["hasMore"].(bool); upstream {
			more = true
		}
	}
	if container == nil {
		return output, nil
	}
	container["hasMore"] = more
	if tabular {
		matrix := make([][]any, 0, len(output))
		for _, r := range output {
			cells := []any{}
			for _, c := range outputColumns {
				cells = append(cells, r[c["name"].(string)])
			}
			matrix = append(matrix, cells)
		}
		container["columns"] = outputColumns
		container["rows"] = matrix
	} else {
		container["rows"] = output
	}
	return raw, nil
}
func projectionFields(p *Projection) []Field {
	r := append([]Field{}, p.Fields...)
	r = append(r, p.Dimensions...)
	return append(r, p.Measures...)
}
func scalar(v any) bool {
	switch v.(type) {
	case string, float64, bool:
		return true
	}
	return false
}
func compare(a, b any) int {
	if x, ok := a.(float64); ok {
		y, _ := b.(float64)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		return 0
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}
func predicate(p Predicate, schema map[string]map[string]any, leaves *int, depth int) (func(map[string]any) bool, error) {
	if depth > 100 {
		return nil, fmt.Errorf("predicate depth limit exceeded")
	}
	groups := 0
	if p.And != nil {
		groups++
	}
	if p.Or != nil {
		groups++
	}
	if p.Not != nil {
		groups++
	}
	if p.Field != "" {
		groups++
	}
	if groups != 1 || (p.Field == "" && (p.Op != "" || p.Value != nil)) {
		return nil, fmt.Errorf("predicate must have exactly one group or leaf")
	}
	if p.Not != nil {
		fn, e := predicate(*p.Not, schema, leaves, depth+1)
		if e != nil {
			return nil, e
		}
		return func(r map[string]any) bool { return !fn(r) }, nil
	}
	if p.And != nil || p.Or != nil {
		children := p.And
		if p.Or != nil {
			children = p.Or
		}
		if len(children) == 0 {
			return nil, fmt.Errorf("empty predicate group")
		}
		fns := []func(map[string]any) bool{}
		for _, child := range children {
			fn, e := predicate(child, schema, leaves, depth+1)
			if e != nil {
				return nil, e
			}
			fns = append(fns, fn)
		}
		return func(r map[string]any) bool {
			for _, fn := range fns {
				if p.And != nil && !fn(r) {
					return false
				}
				if p.Or != nil && fn(r) {
					return true
				}
			}
			return p.And != nil
		}, nil
	}
	*leaves++
	if *leaves > 100 {
		return nil, fmt.Errorf("predicate leaf limit exceeded")
	}
	if schema[p.Field] == nil {
		return nil, fmt.Errorf("unknown filter field %q", p.Field)
	}
	switch p.Op {
	case "isNull":
		return func(r map[string]any) bool { return r[p.Field] == nil }, nil
	case "isNotNull":
		return func(r map[string]any) bool { return r[p.Field] != nil }, nil
	}
	values := []any{p.Value}
	switch p.Op {
	case "eq", "neq", "lt", "lte", "gt", "gte", "contains", "startsWith", "endsWith":
	case "in", "notIn", "between":
		var ok bool
		values, ok = p.Value.([]any)
		if !ok || (p.Op == "between" && len(values) != 2) {
			return nil, fmt.Errorf("operator requires an array; between needs two bounds")
		}
	default:
		return nil, fmt.Errorf("unsupported filter operator %q", p.Op)
	}
	for _, v := range values {
		if v == nil || !scalar(v) {
			return nil, fmt.Errorf("comparison requires non-null scalar values")
		}
		kind, _ := schema[p.Field]["type"].(string)
		actual := valueType(v)
		if kind == "mixed" || kind == "array" || kind == "object" {
			return nil, fmt.Errorf("filter column must have a consistent scalar type")
		}
		if kind == "integer" {
			n, ok := v.(float64)
			if !ok || math.Trunc(n) != n {
				return nil, fmt.Errorf("filter value must be an integer")
			}
		} else if kind != "" && kind != "date" && kind != "timestamp" && kind != actual {
			return nil, fmt.Errorf("filter value type does not match column %s", p.Field)
		}
		if (kind == "date" || kind == "timestamp") && actual != "string" {
			return nil, fmt.Errorf("date/time filter values must be strings")
		}
		if p.Op == "contains" || p.Op == "startsWith" || p.Op == "endsWith" {
			if actual != "string" || (kind != "" && kind != "string") {
				return nil, fmt.Errorf("string operator requires a string column and value")
			}
		}
		if actual == "boolean" && p.Op != "eq" && p.Op != "neq" && p.Op != "in" && p.Op != "notIn" {
			return nil, fmt.Errorf("boolean ordering is unsupported")
		}
	}
	return func(r map[string]any) bool {
		a := r[p.Field]
		if a == nil {
			return false
		}
		compatible := func(b any) bool { return reflect.TypeOf(a) == reflect.TypeOf(b) }
		switch p.Op {
		case "eq":
			return reflect.DeepEqual(a, p.Value)
		case "neq":
			return compatible(p.Value) && !reflect.DeepEqual(a, p.Value)
		case "in", "notIn":
			found := false
			for _, v := range values {
				if reflect.DeepEqual(a, v) {
					found = true
				}
			}
			return found == (p.Op == "in")
		case "between":
			return compatible(values[0]) && compatible(values[1]) && compare(a, values[0]) >= 0 && compare(a, values[1]) <= 0
		case "contains", "startsWith", "endsWith":
			x, ok := a.(string)
			y, ok2 := p.Value.(string)
			if !ok || !ok2 {
				return false
			}
			if p.Op == "contains" {
				return strings.Contains(x, y)
			}
			if p.Op == "startsWith" {
				return strings.HasPrefix(x, y)
			}
			return strings.HasSuffix(x, y)
		}
		if !compatible(p.Value) {
			return false
		}
		cmp := compare(a, p.Value)
		switch p.Op {
		case "lt":
			return cmp < 0
		case "lte":
			return cmp <= 0
		case "gt":
			return cmp > 0
		case "gte":
			return cmp >= 0
		}
		return false
	}, nil
}

func valueType(v any) string {
	switch v.(type) {
	case nil:
		return ""
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "mixed"
}

package preview

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
)

func (p *Package) DescribeDataSource(id string) (Object, error) {
	f, ok := p.fixtures[id]
	if !ok {
		source, exists := p.Sources[id]
		if !exists {
			return nil, fail("MissingDataSource", id, "", "unknown datasource")
		}
		return clone(Object{"id": id, "columns": source.Columns, "capabilities": p.Extension.DataSources[id].Query, "resultContract": source.ResultContract}), nil
	}
	return clone(Object{"id": id, "columns": f.columns, "capabilities": p.Extension.DataSources[id].Query, "resultContract": p.Sources[id].ResultContract}), nil
}
func (p *Package) Execute(id string, q Query) (*Result, error) {
	if p.executeRemote != nil {
		return p.executeRemote(id, q)
	}
	if p.queries != nil {
		p.queries <- struct{}{}
		defer func() { <-p.queries }()
	}
	f, ok := p.fixtures[id]
	if !ok {
		return nil, fail("MissingDataSource", id, "", "unknown datasource")
	}
	d := p.Extension.DataSources[id]
	if q.DataSource != "" && q.DataSource != id {
		return nil, fail("MissingDataSource", id, "dataSource", "request datasource mismatch")
	}
	capability := func(want, allowed bool, name string) error {
		if want && !allowed {
			return fail("UnsupportedCapability", id, name, "capability is not enabled")
		}
		return nil
	}
	for _, check := range []struct {
		want, allowed bool
		name          string
	}{{q.Projection != nil, d.Query.Projection, "projection"}, {q.Filter != nil, d.Query.Filtering, "filtering"}, {len(q.OrderBy) > 0, d.Query.Sorting, "sorting"}, {q.Page != nil, d.Query.Pagination == "offset", "pagination"}} {
		if err := capability(check.want, check.allowed, check.name); err != nil {
			return nil, err
		}
	}
	schema := map[string]Object{}
	for _, c := range f.columns {
		schema[str(c["name"])] = c
	}
	var pred func(Object) bool
	if q.Filter != nil {
		leaves := 0
		var err error
		pred, err = compilePredicate(id, *q.Filter, schema, d, &leaves, 0)
		if leaves > p.limits.PredicateLeaves {
			return nil, fail("LimitExceeded", id, "filter", "host predicate limit exceeded")
		}
		if err != nil {
			return nil, err
		}
	}
	fields := []Field{}
	group := false
	if q.Projection != nil {
		if len(q.Projection.Dimensions) > p.limits.GroupingDimensions {
			return nil, fail("LimitExceeded", id, "projection.dimensions", "maximum grouping dimensions exceeded")
		}
		if len(q.Projection.Fields) > 0 {
			if len(q.Projection.Dimensions) > 0 || len(q.Projection.Measures) > 0 {
				return nil, fail("UnknownColumn", id, "projection", "use fields or dimensions/measures, not both")
			}
			for _, f := range q.Projection.Fields {
				if f.Aggregation != "" && f.Aggregation != "none" {
					return nil, fail("UnsafeAggregation", id, f.Field, "fields projection cannot aggregate")
				}
			}
			fields = append(fields, q.Projection.Fields...)
		}
		fields = append(fields, q.Projection.Dimensions...)
		fields = append(fields, q.Projection.Measures...)
		if len(fields) == 0 {
			return nil, fail("UnknownColumn", id, "projection", "projection cannot be empty")
		}
		for _, m := range q.Projection.Measures {
			allowed, ok := d.Measures[m.Field]
			if !ok {
				return nil, fail("UnsafeAggregation", id, m.Field, "measure is undeclared")
			}
			if m.Aggregation != "" && m.Aggregation != "none" {
				if m.Aggregation != allowed.Aggregation {
					return nil, fail("UnsafeAggregation", id, m.Field, "aggregation is not declared")
				}
				group = true
			}
		}
		if group && !d.Query.Grouping {
			return nil, fail("UnsupportedCapability", id, "grouping", "grouping is disabled")
		}
		if group {
			for _, m := range q.Projection.Measures {
				if m.Aggregation == "" || m.Aggregation == "none" {
					return nil, fail("UnsafeAggregation", id, m.Field, "non-aggregated measure in grouped query")
				}
			}
		}
		for _, dim := range q.Projection.Dimensions {
			if !contains(d.Dimensions, dim.Field) {
				return nil, fail("UnknownColumn", id, dim.Field, "dimension is undeclared")
			}
		}
	} else {
		for _, c := range f.columns {
			fields = append(fields, Field{Field: str(c["name"])})
		}
	}
	if len(fields) > p.limits.Columns {
		return nil, fail("LimitExceeded", id, "projection", "too many output columns")
	}
	columns := []Object{}
	names := map[string]bool{}
	for _, field := range fields {
		c := schema[field.Field]
		if c == nil {
			return nil, fail("UnknownColumn", id, field.Field, "unknown projected column")
		}
		if names[field.name()] {
			return nil, fail("UnknownColumn", id, field.name(), "duplicate output column; supply a unique alias")
		}
		names[field.name()] = true
		c = clone(c)
		c["name"] = field.name()
		if group && field.Aggregation != "" {
			c["nullable"] = true
		}
		if field.Aggregation == "count" || field.Aggregation == "countDistinct" {
			c["type"] = "integer"
			c["nullable"] = false
		}
		if field.Aggregation == "average" {
			c["type"] = "number"
		}
		columns = append(columns, c)
	}
	for _, order := range q.OrderBy {
		var c Object
		for _, col := range columns {
			if col["name"] == order.Field {
				c = col
			}
		}
		if c == nil {
			c = schema[order.Field]
		}
		if contains([]string{"array", "object", "decimal"}, str(c["type"])) {
			return nil, fail("InvalidSort", id, order.Field, "sorting unsupported for column type")
		}
		if order.Direction != "asc" && order.Direction != "desc" {
			return nil, fail("InvalidSort", id, order.Field, "direction must be asc or desc")
		}
		if order.Nulls != "" && order.Nulls != "first" && order.Nulls != "last" {
			return nil, fail("InvalidSort", id, order.Field, "nulls must be first or last")
		}
		if !names[order.Field] && (!contains(d.HiddenSortFields, order.Field) || schema[order.Field] == nil || group) {
			return nil, fail("InvalidSort", id, order.Field, "sort field must be projected or a declared ungrouped hidden field")
		}
	}
	limit, offset := p.Extension.MaxPageSize, 0
	if q.Page != nil {
		limit, offset = q.Page.Limit, q.Page.Offset
		if limit <= 0 || offset < 0 {
			return nil, fail("InvalidPage", id, "page", "positive limit and nonnegative offset required")
		}
		if limit > p.Extension.MaxPageSize {
			limit = p.Extension.MaxPageSize
		}
	}
	b, _ := json.Marshal(q)
	fingerprint := hash(append([]byte(p.definitionHash+":"+f.hash+":"+p.Variant+":"+id+":"), b...))
	if f.errorResponse {
		return &Result{Response: clone(f.raw), Columns: clone(f.columns), Rows: []Object{}, Fingerprint: fingerprint}, nil
	}
	rows := []Object{}
	for _, r := range f.rows {
		if pred == nil || pred(r) {
			rows = append(rows, r)
		}
	}
	if group {
		buckets := map[string][]Object{}
		keys := []string{}
		for _, r := range rows {
			key := []any{}
			for _, dim := range q.Projection.Dimensions {
				key = append(key, r[dim.Field])
			}
			b, _ := json.Marshal(key)
			k := string(b)
			if _, ok := buckets[k]; !ok {
				keys = append(keys, k)
			}
			buckets[k] = append(buckets[k], r)
		}
		if len(rows) == 0 && len(q.Projection.Dimensions) == 0 {
			keys = []string{"[]"}
			buckets["[]"] = nil
		}
		grouped := []Object{}
		for _, k := range keys {
			bucket := buckets[k]
			r := Object{}
			for _, dim := range q.Projection.Dimensions {
				r[dim.Field] = bucket[0][dim.Field]
			}
			for _, m := range q.Projection.Measures {
				v, err := aggregate(id, m, bucket, schema[m.Field])
				if err != nil {
					return nil, err
				}
				r[m.name()] = v
			}
			grouped = append(grouped, r)
		}
		rows = grouped
	}
	projected := []Object{}
	for _, r := range rows {
		out := Object{}
		for _, field := range fields {
			key := field.Field
			if group && field.Aggregation != "" && field.Aggregation != "none" {
				key = field.name()
			}
			out[field.name()] = r[key]
		}
		for _, hidden := range d.HiddenSortFields {
			if !names[hidden] {
				out[hidden] = r[hidden]
			}
		}
		projected = append(projected, out)
	}
	sort.SliceStable(projected, func(i, j int) bool {
		for _, o := range q.OrderBy {
			a, b := projected[i][o.Field], projected[j][o.Field]
			if a == nil || b == nil {
				if a == nil && b == nil {
					continue
				}
				if o.Nulls == "first" {
					return a == nil
				}
				return b == nil
			}
			var column Object
			for _, candidate := range columns {
				if candidate["name"] == o.Field {
					column = candidate
					break
				}
			}
			if column == nil {
				column = schema[o.Field]
			}
			c := compareTyped(a, b, column)
			if c != 0 {
				if o.Direction == "desc" {
					return c > 0
				}
				return c < 0
			}
		}
		return false
	})
	total := len(projected)
	start := min(offset, total)
	end := start + min(limit, total-start)
	more := end < total || f.hasMore
	selected := projected[start:end]
	for _, r := range selected {
		for _, h := range d.HiddenSortFields {
			if !names[h] {
				delete(r, h)
			}
		}
	}
	response := encodeResponse(f.raw, p.Sources[id].ResultContract, d, columns, selected, more, total)
	return &Result{Response: response, Columns: columns, Rows: clone(selected), HasMore: more, Fingerprint: fingerprint}, nil
}
func contains(a []string, v string) bool {
	for _, s := range a {
		if s == v {
			return true
		}
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
	if x, ok := a.(string); ok {
		y, _ := b.(string)
		return strings.Compare(x, y)
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}
func aggregate(id string, m Field, rows []Object, column Object) (any, error) {
	values := []any{}
	for _, r := range rows {
		if r[m.Field] != nil {
			values = append(values, r[m.Field])
		}
	}
	if m.Aggregation == "count" {
		return float64(len(values)), nil
	}
	if m.Aggregation == "countDistinct" {
		seen := map[string]bool{}
		for _, v := range values {
			b, _ := json.Marshal(v)
			seen[string(b)] = true
		}
		return float64(len(seen)), nil
	}
	if len(values) == 0 {
		return nil, nil
	}
	if m.Aggregation == "min" || m.Aggregation == "max" {
		result := values[0]
		for _, v := range values[1:] {
			c := compareTyped(v, result, column)
			if (m.Aggregation == "min" && c < 0) || (m.Aggregation == "max" && c > 0) {
				result = v
			}
		}
		return result, nil
	}
	sum := 0.0
	for _, v := range values {
		n, ok := v.(float64)
		if !ok {
			return nil, fail("UnsafeAggregation", id, m.Field, "numeric aggregation requires numeric values")
		}
		sum += n
	}
	if m.Aggregation == "average" {
		sum /= float64(len(values))
	}
	if math.IsInf(sum, 0) || math.IsNaN(sum) || (column["type"] == "integer" && m.Aggregation != "average" && math.Abs(sum) > 9007199254740991) {
		return nil, fail("TypeMismatch", id, m.Field, "aggregation exceeds numeric precision")
	}
	return sum, nil
}
func compilePredicate(id string, p Predicate, schema map[string]Object, d Declaration, leaves *int, depth int) (func(Object) bool, error) {
	if depth > 100 {
		return nil, fail("LimitExceeded", id, "filter", "predicate depth exceeded")
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
	if (p.Field == "" && (p.Op != "" || p.Value != nil)) || groups != 1 {
		return nil, fail("UnsupportedOperator", id, "filter", "predicate requires exactly one group or leaf")
	}
	if p.Not != nil {
		fn, err := compilePredicate(id, *p.Not, schema, d, leaves, depth+1)
		if err != nil {
			return nil, err
		}
		return func(r Object) bool { return !fn(r) }, nil
	}
	if p.And != nil || p.Or != nil {
		children := p.And
		if p.Or != nil {
			children = p.Or
		}
		if len(children) == 0 {
			return nil, fail("UnsupportedOperator", id, "filter", "empty group")
		}
		fns := []func(Object) bool{}
		for _, child := range children {
			fn, err := compilePredicate(id, child, schema, d, leaves, depth+1)
			if err != nil {
				return nil, err
			}
			fns = append(fns, fn)
		}
		return func(r Object) bool {
			for _, fn := range fns {
				v := fn(r)
				if p.And != nil && !v {
					return false
				}
				if p.Or != nil && v {
					return true
				}
			}
			return p.And != nil
		}, nil
	}
	*leaves++
	if *leaves > 100 {
		return nil, fail("LimitExceeded", id, "filter", "too many predicate leaves")
	}
	c := schema[p.Field]
	if c == nil {
		return nil, fail("UnknownColumn", id, p.Field, "unknown filter field")
	}
	switch p.Op {
	case "isNull":
		return func(r Object) bool { return r[p.Field] == nil }, nil
	case "isNotNull":
		return func(r Object) bool { return r[p.Field] != nil }, nil
	}
	values := []any{p.Value}
	switch p.Op {
	case "eq", "neq":
	case "in", "notIn", "between":
		a, ok := p.Value.([]any)
		if !ok || (p.Op == "between" && len(a) != 2) {
			return nil, fail("UnsupportedOperator", id, p.Field, "operator requires array (between requires two bounds)")
		}
		values = a
	case "lt", "lte", "gt", "gte":
	case "contains", "startsWith", "endsWith":
		if c["type"] != "string" {
			return nil, fail("UnsupportedOperator", id, p.Field, "string operator requires string column")
		}
	default:
		return nil, fail("UnsupportedOperator", id, p.Field, "unknown operator")
	}
	if c["type"] == "array" || c["type"] == "object" || c["type"] == "decimal" {
		return nil, fail("UnsupportedOperator", id, p.Field, "comparison unsupported for column type")
	}
	if c["type"] == "boolean" && contains([]string{"lt", "lte", "gt", "gte", "between"}, p.Op) {
		return nil, fail("UnsupportedOperator", id, p.Field, "ordered comparison unsupported for boolean")
	}
	for _, v := range values {
		if v == nil || !typed(v, c) {
			return nil, fail("TypeMismatch", id, p.Field, "filter value must match column type; use null operators")
		}
	}
	normalize := func(v any) any {
		if d.CaseSensitive != nil && !*d.CaseSensitive {
			if s, ok := v.(string); ok {
				return strings.ToLower(s)
			}
		}
		return v
	}
	return func(r Object) bool {
		a := normalize(r[p.Field])
		if a == nil {
			return false
		}
		v := normalize(p.Value)
		cmp := compareTyped(a, v, c)
		switch p.Op {
		case "eq":
			return equalTyped(a, v, c)
		case "neq":
			return !equalTyped(a, v, c)
		case "lt":
			return cmp < 0
		case "lte":
			return cmp <= 0
		case "gt":
			return cmp > 0
		case "gte":
			return cmp >= 0
		case "in", "notIn":
			found := false
			for _, candidate := range values {
				if equalTyped(a, normalize(candidate), c) {
					found = true
					break
				}
			}
			return found == (p.Op == "in")
		case "between":
			return compareTyped(a, normalize(values[0]), c) >= 0 && compareTyped(a, normalize(values[1]), c) <= 0
		case "contains":
			return strings.Contains(str(a), str(v))
		case "startsWith":
			return strings.HasPrefix(str(a), str(v))
		case "endsWith":
			return strings.HasSuffix(str(a), str(v))
		}
		return false
	}, nil
}
func setAt(root Object, path string, value any) {
	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	for _, key := range parts[:len(parts)-1] {
		next, ok := root[key].(map[string]any)
		if !ok {
			next = Object{}
			root[key] = next
		}
		root = next
	}
	root[parts[len(parts)-1]] = value
}
func encodeResponse(raw any, contract Contract, d Declaration, columns []Object, rows []Object, more bool, total int) any {
	if contract.Shape == "" {
		contract = d.InputContract
	}
	if contract.Shape == "records" && contract.RowPath == "$" {
		return clone(rows)
	}
	response, ok := clone(raw).(map[string]any)
	if !ok {
		response = Object{"status": "ok"}
	}
	if contract.Shape == "records" {
		setAt(response, contract.RowPath, rows)
		if contract.HasMorePath != "" {
			setAt(response, contract.HasMorePath, more)
		}
	} else {
		path := contract.ResultsPath
		if path == "" {
			path = "data"
		}
		name := contract.ResultName
		if name == "" {
			name = d.ResultName
		}
		matrix := make([][]any, 0, len(rows))
		for _, r := range rows {
			v := make([]any, 0, len(columns))
			for _, c := range columns {
				v = append(v, r[str(c["name"])])
			}
			matrix = append(matrix, v)
		}
		result := Object{"name": name, "columns": columns, "rows": matrix, "hasMore": more}
		results, ok := at(response, path).([]any)
		if !ok {
			results = []any{result}
		} else {
			found := false
			for i, v := range results {
				if obj, ok := v.(map[string]any); ok && obj["name"] == name {
					for k, value := range result {
						obj[k] = value
					}
					results[i] = obj
					found = true
				}
			}
			if !found {
				results = append(results, result)
			}
		}
		setAt(response, path, results)
	}
	if d.IncludeTotalRows {
		meta, ok := response["meta"].(map[string]any)
		if !ok {
			meta = Object{}
			response["meta"] = meta
		}
		meta["totalRows"] = total
	}
	return response
}

func compareTyped(a, b any, column Object) int {
	if column["type"] == "timestamp" {
		x, ex := time.Parse(time.RFC3339Nano, str(a))
		y, ey := time.Parse(time.RFC3339Nano, str(b))
		if ex == nil && ey == nil {
			if x.Before(y) {
				return -1
			}
			if x.After(y) {
				return 1
			}
			return 0
		}
	}
	return compare(a, b)
}
func equalTyped(a, b any, column Object) bool {
	if column["type"] == "timestamp" {
		return compareTyped(a, b, column) == 0
	}
	return reflect.DeepEqual(a, b)
}

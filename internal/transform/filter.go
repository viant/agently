package transform

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Spec defines a simple JSON post-filter applied to tool outputs.
// Supported keys in selector suffix: root, where, sort, limit, select.
type Spec struct {
	Root   string
	Where  string
	Sort   []SortKey
	Limit  int
	Select []string
}

type SortKey struct {
	Field string
	Desc  bool
}

// ParseSuffix parses a selector suffix like "root=$.items; where=name~=review|critique; sort=-rank,name; limit=5; select=id,name".
func ParseSuffix(s string) *Spec {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	spec := &Spec{}
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		switch key {
		case "root":
			spec.Root = val
		case "where":
			spec.Where = val
		case "sort":
			fields := strings.Split(val, ",")
			for _, f := range fields {
				f = strings.TrimSpace(f)
				if f == "" {
					continue
				}
				desc := strings.HasPrefix(f, "-")
				field := strings.TrimPrefix(f, "-")
				spec.Sort = append(spec.Sort, SortKey{Field: field, Desc: desc})
			}
		case "limit", "topn":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				spec.Limit = n
			}
		case "select":
			cols := strings.Split(val, ",")
			for _, c := range cols {
				c = strings.TrimSpace(c)
				if c != "" {
					spec.Select = append(spec.Select, c)
				}
			}
		}
	}
	return spec
}

// Apply filters the input JSON bytes based on the spec and returns filtered JSON bytes.
func (s *Spec) Apply(data []byte) ([]byte, error) {
	if s == nil {
		return data, nil
	}
	var root interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		// not JSON → return as-is
		return data, nil
	}
	// Resolve root array
	arr := extractArray(root, s.Root)
	if arr == nil {
		// no array at root; return original
		return data, nil
	}
	// Filter
	filtered := make([]map[string]interface{}, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		if s.matchWhere(m) {
			filtered = append(filtered, m)
		}
	}
	// Sort
	if len(s.Sort) > 0 {
		sort.SliceStable(filtered, func(i, j int) bool {
			for _, k := range s.Sort {
				vi := toString(deepGet(filtered[i], k.Field))
				vj := toString(deepGet(filtered[j], k.Field))
				if vi == vj {
					continue
				}
				if k.Desc {
					return strings.ToLower(vi) > strings.ToLower(vj)
				}
				return strings.ToLower(vi) < strings.ToLower(vj)
			}
			return false
		})
	}
	// Limit
	if s.Limit > 0 && len(filtered) > s.Limit {
		filtered = filtered[:s.Limit]
	}
	// Select
	out := make([]interface{}, 0, len(filtered))
	if len(s.Select) > 0 {
		for _, m := range filtered {
			proj := map[string]interface{}{}
			for _, f := range s.Select {
				proj[f] = deepGet(m, f)
			}
			out = append(out, proj)
		}
	} else {
		for _, m := range filtered {
			out = append(out, m)
		}
	}
	return json.Marshal(out)
}

func extractArray(root interface{}, path string) []interface{} {
	// default: if root is array, return it; if object with items, return items
	if strings.TrimSpace(path) == "" {
		switch v := root.(type) {
		case []interface{}:
			return v
		case map[string]interface{}:
			if items, ok := v["items"].([]interface{}); ok {
				return items
			}
		}
		return nil
	}
	// support $.items or $ for root
	path = strings.TrimSpace(path)
	if path == "$" {
		if v, ok := root.([]interface{}); ok {
			return v
		}
		return nil
	}
	if strings.HasPrefix(path, "$.") {
		key := strings.TrimPrefix(path, "$.")
		if m, ok := root.(map[string]interface{}); ok {
			if v, ok := m[key]; ok {
				if arr, ok := v.([]interface{}); ok {
					return arr
				}
			}
		}
	}
	return nil
}

func deepGet(m map[string]interface{}, field string) interface{} {
	parts := strings.Split(field, ".")
	cur := interface{}(m)
	for _, p := range parts {
		if mm, ok := cur.(map[string]interface{}); ok {
			cur = mm[p]
		} else {
			return nil
		}
	}
	return cur
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (s *Spec) matchWhere(m map[string]interface{}) bool {
	w := strings.TrimSpace(s.Where)
	if w == "" {
		return true
	}
	// Very small parser: support name=csv, name~=regex (OR), contains(tags,'v'), rank>=N
	clauses := strings.Split(w, "&&")
	for _, c := range clauses {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.Contains(c, "~=") {
			kv := strings.SplitN(c, "~=", 2)
			field := strings.TrimSpace(kv[0])
			pat := strings.TrimSpace(kv[1])
			val := toString(deepGet(m, field))
			// Support OR with '|' or ','
			parts := strings.FieldsFunc(pat, func(r rune) bool { return r == ',' || r == '|' })
			matched := false
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				re, err := regexp.Compile(p)
				if err != nil {
					return false
				}
				if re.MatchString(val) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
			continue
		}
		if strings.Contains(c, "=") && !strings.Contains(c, ">=") && !strings.Contains(c, "<=") {
			kv := strings.SplitN(c, "=", 2)
			field := strings.TrimSpace(kv[0])
			vals := strings.Split(kv[1], ",")
			val := strings.ToLower(strings.TrimSpace(toString(deepGet(m, field))))
			ok := false
			for _, v := range vals {
				if strings.ToLower(strings.TrimSpace(v)) == val {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(c), "contains(") {
			// contains(field,'value')
			inner := strings.TrimSuffix(strings.TrimPrefix(c, "contains("), ")")
			args := strings.SplitN(inner, ",", 2)
			if len(args) != 2 {
				return false
			}
			field := strings.TrimSpace(args[0])
			needle := strings.Trim(strings.TrimSpace(args[1]), "'\"")
			v := deepGet(m, field)
			switch arr := v.(type) {
			case []interface{}:
				found := false
				for _, it := range arr {
					if strings.EqualFold(toString(it), needle) {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			case string:
				if !strings.Contains(strings.ToLower(arr), strings.ToLower(needle)) {
					return false
				}
			default:
				return false
			}
			continue
		}
		if strings.Contains(c, ">=") {
			kv := strings.SplitN(c, ">=", 2)
			field := strings.TrimSpace(kv[0])
			cutoff, _ := strconv.Atoi(strings.TrimSpace(kv[1]))
			v := deepGet(m, field)
			num := 0
			switch t := v.(type) {
			case float64:
				num = int(t)
			case int:
				num = t
			case string:
				num, _ = strconv.Atoi(strings.TrimSpace(t))
			}
			if num < cutoff {
				return false
			}
			continue
		}
		// unknown clause → ignore (be conservative)
	}
	return true
}

package refiner

import (
	"github.com/viant/mcp-protocol/schema"
	"sort"
	"strings"
)

func Refine(rs *schema.ElicitRequestParamsRequestedSchema) {
	if rs == nil {
		return
	}
	applyPreset(rs)
	for key, val := range rs.Properties {
		prop, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := prop["type"]; !has {
			prop["type"] = "string"
		}
		// Ensure sensible defaults and UI hints per type
		if t, _ := prop["type"].(string); t == "array" {
			// Default empty array instead of object to avoid UI initializing to {}
			if _, has := prop["default"]; !has {
				prop["default"] = []any{}
			}
			// When items are strings, prefer a tags-style widget for better UX
			if it, ok := prop["items"].(map[string]any); ok {
				if itType, _ := it["type"].(string); itType == "string" {
					if _, has := prop["x-ui-widget"]; !has {
						prop["x-ui-widget"] = "tags"
					}
				}
			}
		}
		if _, has := prop["format"]; !has {
			hint := strings.ToLower(convToString(prop["title"]) + " " + convToString(prop["description"]))
			if strings.Contains(hint, "yyyy-mm-dd") || strings.Contains(hint, "yyyy/mm/dd") {
				if strings.Contains(hint, "hh") || strings.Contains(hint, "hour") || strings.Contains(hint, "mm") || strings.Contains(hint, "ss") || strings.Contains(hint, "time") {
					prop["format"] = "date-time"
				} else {
					prop["format"] = "date"
				}
			}
		}
		rs.Properties[key] = prop
	}
	if !hasExplicitOrder(rs) {
		assignAutoOrder(rs)
	}
}

func hasExplicitOrder(rs *schema.ElicitRequestParamsRequestedSchema) bool {
	for _, v := range rs.Properties {
		if m, ok := v.(map[string]interface{}); ok {
			if _, ok2 := m["x-ui-order"]; ok2 {
				return true
			}
		}
	}
	return false
}

func assignAutoOrder(rs *schema.ElicitRequestParamsRequestedSchema) {
	seen := map[string]struct{}{}
	orderKeys := []string{}
	for k := range rs.Properties {
		if strings.HasPrefix(strings.ToLower(k), "name") {
			orderKeys = append(orderKeys, k)
			seen[k] = struct{}{}
		}
	}
	for _, r := range rs.Required {
		if _, ok := rs.Properties[r]; ok {
			if _, dup := seen[r]; !dup {
				orderKeys = append(orderKeys, r)
				seen[r] = struct{}{}
			}
		}
	}
	for k := range rs.Properties {
		if _, dup := seen[k]; dup {
			continue
		}
		lk := strings.ToLower(k)
		if strings.Contains(lk, "start") {
			orderKeys = append(orderKeys, k)
			seen[k] = struct{}{}
			base := strings.ReplaceAll(strings.ReplaceAll(lk, "start", ""), "_", "")
			for cand := range rs.Properties {
				if _, dup2 := seen[cand]; dup2 {
					continue
				}
				lc := strings.ToLower(cand)
				if strings.Contains(lc, "end") {
					stripped := strings.ReplaceAll(strings.ReplaceAll(lc, "end", ""), "_", "")
					if stripped == base {
						orderKeys = append(orderKeys, cand)
						seen[cand] = struct{}{}
						break
					}
				}
			}
		}
	}
	rest := []string{}
	for k := range rs.Properties {
		if _, ok := seen[k]; !ok {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	orderKeys = append(orderKeys, rest...)
	seq := 10
	for _, k := range orderKeys {
		if prop, ok := rs.Properties[k].(map[string]interface{}); ok {
			prop["x-ui-order"] = seq
			seq += 10
			rs.Properties[k] = prop
		}
	}
}

func convToString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

package refiner

// Helper that post-processes an ElicitRequestParamsRequestedSchema so that UI
// form generators (Forge) get richer hints: explicit type, layout order, date
// detection, column span.

import (
	"sort"
	"strings"

	"github.com/viant/mcp-protocol/schema"
)

// Refine mutates the supplied schema in-place. It is safe to call multiple
// times (idempotent for the generated hints).
func Refine(rs *schema.ElicitRequestParamsRequestedSchema) {
	if rs == nil {
		return
	}

	// Apply user-configured global preset before the default heuristics so
	// that explicit x-ui-order markers prevent the auto-order block from
	// kicking in later.
	applyPreset(rs)

	// ------------------------------------------------------------------
	// First pass – ensure each property has minimal metadata (type, formats).
	// ------------------------------------------------------------------
	for key, val := range rs.Properties {
		prop, ok := val.(map[string]interface{})
		if !ok {
			// not a map → cannot mutate reliably
			continue
		}

		// Guarantee "type" so that form renderer falls back to text input.
		if _, has := prop["type"]; !has {
			prop["type"] = "string"
		}

		// Detect date / date-time from description or title when format
		// missing.  Very simple heuristic but good enough for common prompts.
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

	// ------------------------------------------------------------------
	// Ordering – honour explicit x-ui-order, otherwise compute default.
	// ------------------------------------------------------------------
	explicit := hasExplicitOrder(rs)
	if !explicit {
		assignAutoOrder(rs)
	}
}

// hasExplicitOrder checks whether at least one property already defines
// x-ui-order – in that case we respect caller’s ordering.
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

// assignAutoOrder generates x-ui-order values (increments of 10) following
// heuristics:
//  1. Fields named "name*" first.
//  2. Required list (original order).
//  3. Pair “…start…” with matching “…end…”.
//  4. Remaining keys alphabetical.
func assignAutoOrder(rs *schema.ElicitRequestParamsRequestedSchema) {
	seen := map[string]struct{}{}
	orderKeys := []string{}

	// 1. name* prefix
	for k := range rs.Properties {
		if strings.HasPrefix(strings.ToLower(k), "name") {
			orderKeys = append(orderKeys, k)
			seen[k] = struct{}{}
		}
	}

	// 2. required (keep supplied order)
	for _, r := range rs.Required {
		if _, ok := rs.Properties[r]; ok {
			if _, dup := seen[r]; !dup {
				orderKeys = append(orderKeys, r)
				seen[r] = struct{}{}
			}
		}
	}

	// 3. start/end neighbour pairs
	for k := range rs.Properties {
		if _, dup := seen[k]; dup {
			continue
		}
		lk := strings.ToLower(k)
		if strings.Contains(lk, "start") {
			orderKeys = append(orderKeys, k)
			seen[k] = struct{}{}

			base := strings.ReplaceAll(strings.ReplaceAll(lk, "start", ""), "_", "")
			// attempt to find counterpart
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

	// 4. remaining alphabetical.
	rest := []string{}
	for k := range rs.Properties {
		if _, ok := seen[k]; !ok {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	orderKeys = append(orderKeys, rest...)

	// Apply order numbers.
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
	switch s := v.(type) {
	case string:
		return s
	default:
		return ""
	}
}

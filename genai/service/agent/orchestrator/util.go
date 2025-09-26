package orchestrator

import (
	"encoding/json"
	"reflect"
	"sort"
)

// MapEqual reports whether two maps contain the same keys and the corresponding
// values are deeply equal.  Nil and empty maps are treated as equivalent.
//
// Prior implementation ignored type mismatches which could lead to false
// positives (e.g. string "1" vs number 1).  This version returns false on any
// differing key, value or value type.
func MapEqual(a, b map[string]interface{}) bool {
	// Treat nil and empty maps as the same so callers don't have to special-case.
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for k, av := range a {
		bv, ok := b[k]
		if !ok { // key missing in b
			return false
		}

		// Values of different concrete types are not equal.
		if reflect.TypeOf(av) != reflect.TypeOf(bv) {
			return false
		}

		if !reflect.DeepEqual(av, bv) {
			return false
		}
	}
	return true
}

// CanonicalArgs returns a deterministic, canonical JSON representation of the
// provided args map. Keys are sorted recursively so that semantically equal
// argument objects yield identical strings regardless of original key order.
// The empty map or nil returns an empty string for convenience when building
// composite keys.
func CanonicalArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}

	canon := canonicalize(args)
	data, _ := json.Marshal(canon)
	return string(data)
}

// canonicalize walks the value and, for maps, produces a new map with keys
// sorted in a stable order and child values canonicalised recursively. Basic
// values are returned as-is.
func canonicalize(v interface{}) interface{} {
	switch tv := v.(type) {
	case map[string]interface{}:
		if len(tv) == 0 {
			return map[string]interface{}{}
		}
		keys := make([]string, 0, len(tv))
		for k := range tv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(tv))
		for _, k := range keys {
			out[k] = canonicalize(tv[k])
		}
		return out
	case []interface{}:
		arr := make([]interface{}, len(tv))
		for i, el := range tv {
			arr[i] = canonicalize(el)
		}
		return arr
	default:
		return tv
	}
}

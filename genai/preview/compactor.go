package preview

// Display-only JSON compactor. It reduces large payloads to fit a target budget
// while preserving overall structure where practical. It never attempts to
// drive pagination or continuation – use continuation policy for that.

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/viant/agently/shared"
)

// Options controls compaction thresholds.
type Options struct {
	// BudgetBytes is the target maximum serialized JSON size (approximate).
	BudgetBytes int
	// MaxString clamps string lengths (runes). 0 disables string trimming.
	MaxString int
	// MaxArray keeps only first N items and summarizes the rest. 0 disables array compaction.
	MaxArray int
	// PreserveKeys are object keys that should not be summarized when choosing a largest key.
	PreserveKeys []string
	// LowValueKeys are candidates to summarize first when sizes are similar (e.g., "log", "html", "debug").
	LowValueKeys []string
}

// Meta describes the compaction outcome.
type Meta struct {
	Truncated           bool `json:"truncated"`
	ApproxOriginalBytes int  `json:"approxOriginalBytes"`
	ApproxReturnedBytes int  `json:"approxReturnedBytes"`
}

// Compact reduces v to fit within opts.BudgetBytes where possible. It returns
// a new value; the input is not modified. When the result exceeds the budget
// even after compaction, it is returned best-effort with Meta.Truncated=true.
func Compact(v interface{}, opts Options) (interface{}, Meta, error) {
	// Defensive caps
	if opts.BudgetBytes <= 0 {
		// default ~64KB
		opts.BudgetBytes = 64 * 1024
	}
	// Deep copy via JSON round-trip to avoid mutating the caller's data.
	cloned, origBytes, err := cloneAndSize(v)
	if err != nil {
		return nil, Meta{}, err
	}
	if origBytes <= opts.BudgetBytes {
		return cloned, Meta{Truncated: false, ApproxOriginalBytes: origBytes, ApproxReturnedBytes: origBytes}, nil
	}

	cur := cloned
	// First pass: trim long strings and large arrays bottom-up.
	cur = trimStrings(cur, opts.MaxString)
	cur = compactArrays(cur, opts.MaxArray)

	bytesNow := sizeOf(cur)
	// Iteratively summarize largest offenders until under budget (limited steps)
	steps := 0
	for bytesNow > opts.BudgetBytes && steps < 8 {
		cur2, changed := summarizeLargestObjectField(cur, opts)
		if !changed {
			break
		}
		cur = cur2
		bytesNow = sizeOf(cur)
		steps++
	}

	return cur, Meta{Truncated: bytesNow > opts.BudgetBytes, ApproxOriginalBytes: origBytes, ApproxReturnedBytes: bytesNow}, nil
}

// cloneAndSize returns a deep copy via JSON and the serialized size of the original value.
func cloneAndSize(v interface{}) (interface{}, int, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, 0, err
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, 0, err
	}
	return out, len(b), nil
}

func sizeOf(v interface{}) int {
	b, _ := json.Marshal(v)
	return len(b)
}

// trimStrings truncates long strings (in runes) recursively.
func trimStrings(v interface{}, maxRunes int) interface{} {
	if maxRunes <= 0 {
		return v
	}
	switch t := v.(type) {
	case string:
		if runeCount(t) > maxRunes {
			return shared.RuneTruncate(t, maxRunes)
		}
		return v
	case []interface{}:
		out := make([]interface{}, len(t))
		for i := range t {
			out[i] = trimStrings(t[i], maxRunes)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = trimStrings(val, maxRunes)
		}
		return out
	default:
		return v
	}
}

func runeCount(s string) int { return len([]rune(s)) }

// compactArrays keeps the first N items of arrays and summarizes the rest.
func compactArrays(v interface{}, keep int) interface{} {
	if keep <= 0 {
		return v
	}
	switch t := v.(type) {
	case []interface{}:
		if len(t) <= keep {
			// still compact nested arrays
			out := make([]interface{}, len(t))
			for i := range t {
				out[i] = compactArrays(t[i], keep)
			}
			return out
		}
		kept := make([]interface{}, 0, keep)
		for i := 0; i < keep && i < len(t); i++ {
			kept = append(kept, compactArrays(t[i], keep))
		}
		// Replace array with a typed summary that contains head items.
		return map[string]interface{}{
			"__summary":      "array",
			"kept":           kept,
			"__omittedCount": len(t) - keep,
		}
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = compactArrays(val, keep)
		}
		return out
	default:
		return v
	}
}

// summarizeLargestObjectField finds the largest field in objects (preferring
// low-value keys) and replaces its value with a compact summary. Returns the
// modified structure and whether any change occurred.
func summarizeLargestObjectField(v interface{}, opts Options) (interface{}, bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		// Do not alter already summarized nodes
		if _, isSummary := t["__summary"]; isSummary {
			return t, false
		}
		// Recurse to children first
		changed := false
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			nv, ch := summarizeLargestObjectField(val, opts)
			if ch {
				changed = true
			}
			out[k] = nv
		}
		if changed {
			return out, true
		}
		// Select a field to summarize
		type kv struct {
			key  string
			size int
		}
		var items []kv
		pres := toSet(opts.PreserveKeys)
		lows := toSet(opts.LowValueKeys)
		for k, val := range out {
			if strings.HasPrefix(k, "__") {
				continue
			}
			if pres[k] {
				continue
			}
			s := sizeOf(val)
			items = append(items, kv{k, s})
		}
		if len(items) == 0 {
			return out, false
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].size == items[j].size {
				// Prefer low-value keys for equal size
				if lows[items[i].key] && !lows[items[j].key] {
					return true
				}
				if !lows[items[i].key] && lows[items[j].key] {
					return false
				}
				return items[i].key < items[j].key
			}
			return items[i].size > items[j].size
		})
		target := items[0]
		// Summarize target field based on its kind
		switch val := out[target.key].(type) {
		case map[string]interface{}:
			out[target.key] = map[string]interface{}{
				"__summary":   "object",
				"keys":        len(val),
				"approxBytes": target.size,
			}
		case []interface{}:
			// If arrays are still present (keep==0), summarize without kept
			out[target.key] = map[string]interface{}{
				"__summary":   "array",
				"approxBytes": target.size,
				"length":      len(val),
			}
		case string:
			// Trim string further if possible; if no change, signal unchanged so
			// a higher-level summarization can proceed.
			if opts.MaxString > 0 && runeCount(val) > opts.MaxString/2 {
				out[target.key] = shared.RuneTruncate(val, opts.MaxString/2)
				return out, true
			}
			return out, false
		default:
			// Replace bulky scalars with a summary
			out[target.key] = map[string]interface{}{"__summary": "value", "approxBytes": target.size}
		}
		return out, true
	case []interface{}:
		out := make([]interface{}, len(t))
		changed := false
		for i := range t {
			nv, ch := summarizeLargestObjectField(t[i], opts)
			if ch {
				changed = true
			}
			out[i] = nv
		}
		return out, changed
	default:
		return v, false
	}
}

func toSet(ss []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range ss {
		out[s] = true
	}
	return out
}

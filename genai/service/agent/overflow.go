package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// buildOverflowPreview trims body to limit and appends a simple omitted trailer.
// When allowContinuation is true and refMessageID is provided, it will emit a
// continuation wrapper (or JSON truncation) and return overflow=true to enable
// paging via message:show. Otherwise, it returns a plain truncated preview and
// overflow=false so the paging tool is not exposed.
func buildOverflowPreview(body string, threshold int, refMessageID string, allowContinuation bool) (string, bool) {
	body = strings.TrimSpace(body)
	if threshold <= 0 || len(body) <= threshold {
		return body, false
	}
	limit := int(0.9 * float64(threshold)) // to prevent internal show result being over threshold when wrapped as json + metadata

	if allowContinuation && strings.TrimSpace(refMessageID) != "" {
		if jsonPreview, ok := truncateContinuationJSON(body, limit); ok {
			return jsonPreview, true
		}
		size := len(body)
		returned := limit
		if returned > size {
			returned = size
		}
		remaining := size - returned
		chunk := strings.TrimSpace(body[:returned])
		chunk += "[... omitted from " + fmt.Sprintf("%d", returned) + " to " + fmt.Sprintf("%d", size) + "]"
		id := strings.TrimSpace(refMessageID)
		return fmt.Sprintf(`overflow: true
messageId: %s
nextRange:
  bytes:
    offset: %d
    length: %d
hasMore: true
remaining: %d
returned: %d
useToolToSeeMore: internal_message-show
content: |
%s`, id, returned, returned, remaining, returned, chunk), true
	}

	size := len(body)
	returned := limit
	if returned > size {
		returned = size
	}
	chunk := strings.TrimSpace(body[:returned])
	chunk += "[... omitted " + fmt.Sprintf("%d", size-returned) + " of " + fmt.Sprintf("%d", size) + "]"
	return chunk, false
}

// truncateContinuationJSON attempts to truncate the largest string field within
// a JSON object (searching nested objects/arrays) while preserving continuation
// metadata. Returns the modified JSON and true when truncation occurred,
// otherwise false.
func truncateContinuationJSON(body string, limit int) (string, bool) {
	if !strings.HasPrefix(body, "{") {
		return "", false
	}
	var root interface{}
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return "", false
	}
	parent, key, idx, value := findLargestString(root)
	if parent == nil || value == "" || limit <= 0 || len(value) <= limit {
		return "", false
	}
	truncated := strings.TrimSpace(value[:limit])
	switch container := parent.(type) {
	case map[string]interface{}:
		container[key] = truncated
	case []interface{}:
		if idx >= 0 && idx < len(container) {
			container[idx] = truncated
		}
	default:
		return "", false
	}

	remaining := len(value) - len(truncated)
	returned := len(truncated)
	rootMap, ok := root.(map[string]interface{})
	if !ok {
		return "", false
	}
	rootMap["remaining"] = remaining
	rootMap["returned"] = returned

	cont, _ := rootMap["continuation"].(map[string]interface{})
	if cont == nil {
		cont = map[string]interface{}{}
		rootMap["continuation"] = cont
	}
	cont["hasMore"] = true
	cont["remaining"] = remaining
	cont["returned"] = returned
	if _, ok := cont["mode"]; !ok {
		cont["mode"] = "head"
	}
	nextRange, _ := cont["nextRange"].(map[string]interface{})
	if nextRange == nil {
		nextRange = map[string]interface{}{}
	}
	bytesHint, _ := nextRange["bytes"].(map[string]interface{})
	if bytesHint == nil {
		bytesHint = map[string]interface{}{}
	}
	bytesHint["offset"] = returned
	nextLength := returned
	if remaining > 0 && remaining < nextLength {
		nextLength = remaining
	}
	bytesHint["length"] = nextLength
	nextRange["bytes"] = bytesHint
	cont["nextRange"] = nextRange

	out, err := json.Marshal(root)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// findLargestString walks nested maps/slices to locate the largest string value.
// It returns the parent container, map key or slice index, and the string value.
func findLargestString(v interface{}) (parent interface{}, key string, idx int, val string) {
	idx = -1
	var visit func(node interface{}, p interface{}, k string, i int)
	visit = func(node interface{}, p interface{}, k string, i int) {
		switch n := node.(type) {
		case string:
			if len(n) > len(val) {
				parent, key, idx, val = p, k, i, n
			}
		case map[string]interface{}:
			for mk, mv := range n {
				visit(mv, n, mk, -1)
			}
		case []interface{}:
			for si, sv := range n {
				visit(sv, n, "", si)
			}
		}
	}
	visit(v, nil, "", -1)
	return
}

package overflow

// Package overflow contains helpers to build a standard YAML wrapper that
// communicates overflow/continuation hints when native paging isn't available.

import (
	"fmt"
	"strings"

	"github.com/viant/mcp-protocol/extension"
	yaml "gopkg.in/yaml.v3"
)

// BuildOverflowYAML produces a YAML helper document that LLMs and UIs can use
// to continue reading large content via internal_message-show. It includes the
// message id, returned/remaining counts and a nextRange hint when available.
func BuildOverflowYAML(messageID string, cont *extension.Continuation) (string, error) {
	doc := map[string]any{
		"overflow":  true,
		"messageId": strings.TrimSpace(messageID),
	}
	if cont != nil {
		if cont.Returned > 0 {
			doc["returned"] = cont.Returned
		}
		if cont.Remaining > 0 {
			doc["remaining"] = cont.Remaining
		}
		// Prefer bytes nextRange when available
		if cont.NextRange != nil && cont.NextRange.Bytes != nil {
			off := cont.NextRange.Bytes.Offset
			length := cont.NextRange.Bytes.Length
			if off < 0 {
				off = 0
			}
			if length < 0 {
				length = 0
			}
			end := off + length
			doc["nextRange"] = fmt.Sprintf("%d-%d", off, end)
			doc["bytes"] = map[string]int{
				"offset": off,
				"length": length,
			}
		}
	}
	// Generic hint for callers. We keep it short and actionable.
	doc["hint"] = "Call internal_message-show with messageId and byteRange.from/to from nextRange."

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

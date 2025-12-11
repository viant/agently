package executil

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/viant/agently/genai/tool"
	contpol "github.com/viant/agently/genai/tool/continuation"
	overwrap "github.com/viant/agently/genai/tool/overflow"
	schinspect "github.com/viant/agently/genai/tool/schema"
	"github.com/viant/mcp-protocol/extension"
)

// maybeWrapOverflow inspects a tool result for continuation hints and, based on
// input/output schemas, returns a YAML overflow wrapper when native range
// continuation is not supported. When no wrapper is needed, it returns an empty string.
func maybeWrapOverflow(_ context.Context, reg tool.Registry, toolName, result, toolMsgID string) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return ""
	}
	contRaw, ok := payload["continuation"].(map[string]interface{})
	if !ok || contRaw == nil {
		return ""
	}
	hasMore := false
	if v, ok := contRaw["hasMore"]; ok {
		if b, ok2 := v.(bool); ok2 && b {
			hasMore = true
		}
	}
	remaining := intFromAny(contRaw["remaining"])
	if !hasMore && remaining <= 0 {
		return ""
	}
	def, ok := reg.GetDefinition(toolName)
	var inShape schinspect.RangeInputs
	var outShape schinspect.ContinuationShape
	if ok && def != nil {
		_, inShape = schinspect.HasInputRanges(def.Parameters)
		_, outShape = schinspect.HasOutputContinuation(def.OutputSchema)
	}
	strat := contpol.Decide(inShape, outShape)
	switch strat {
	case contpol.OutputOnlyRanges, contpol.NoRanges:
		ext := toContinuation(contRaw)
		yaml, err := overwrap.BuildOverflowYAML(toolMsgID, ext)
		if err == nil && strings.TrimSpace(yaml) != "" {
			return yaml
		}
		return ""
	default:
		return ""
	}
}

func intFromAny(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		if f, err := t.Float64(); err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

// toContinuation converts a generic map continuation into extension.Continuation.
func toContinuation(m map[string]interface{}) *extension.Continuation {
	if m == nil {
		return nil
	}
	c := &extension.Continuation{}
	if v, ok := m["hasMore"].(bool); ok {
		c.HasMore = v
	}
	c.Remaining = intFromAny(m["remaining"])
	c.Returned = intFromAny(m["returned"])
	if nr, ok := m["nextRange"].(map[string]interface{}); ok && nr != nil {
		if b, ok := nr["bytes"].(map[string]interface{}); ok && b != nil {
			off := intFromAny(b["offset"])
			if off == 0 {
				off = intFromAny(b["offsetBytes"])
			}
			ln := intFromAny(b["length"])
			if ln == 0 {
				ln = intFromAny(b["lengthBytes"])
			}
			c.NextRange = &extension.RangeHint{Bytes: &extension.ByteRange{Offset: off, Length: ln}}
		}
	} else if s, ok := m["nextRange"].(string); ok && strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		if len(parts) == 2 {
			var off, end int
			if o, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				off = o
			}
			if e, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				end = e
			}
			ln := 0
			if end > off {
				ln = end - off
			}
			c.NextRange = &extension.RangeHint{Bytes: &extension.ByteRange{Offset: off, Length: ln}}
		}
	}
	return c
}

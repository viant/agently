package sdk

import (
	"strings"
)

// IsPreamble reports whether the event carries a preamble marker.
func IsPreamble(ev *StreamEventEnvelope) bool {
	if ev == nil || ev.Content == nil {
		return false
	}
	meta, ok := ev.Content["meta"].(map[string]interface{})
	if !ok {
		return false
	}
	kind, _ := meta["kind"].(string)
	return strings.EqualFold(strings.TrimSpace(kind), "preamble")
}

// ToolPhase returns "request" or "response" when present.
func ToolPhase(ev *StreamEventEnvelope) string {
	if ev == nil || ev.Content == nil {
		return ""
	}
	meta, ok := ev.Content["meta"].(map[string]interface{})
	if !ok {
		return ""
	}
	phase, _ := meta["phase"].(string)
	return strings.TrimSpace(phase)
}

// ToolPhaseFromEvent maps SSE event names to tool phases when the tool call
// payload is embedded in the message rather than content metadata.
func ToolPhaseFromEvent(ev *StreamEventEnvelope) string {
	if ev == nil {
		return ""
	}
	switch ev.Event.normalize() {
	case StreamEventToolCallStarted:
		return "request"
	case StreamEventToolCallCompleted:
		return "response"
	case StreamEventToolCallFailed:
		return "failed"
	default:
		return ""
	}
}

// ToolName returns the tool name from content when present.
func ToolName(ev *StreamEventEnvelope) string {
	if ev == nil || ev.Content == nil {
		return ""
	}
	name, _ := ev.Content["name"].(string)
	return strings.TrimSpace(name)
}

// ToolCallID returns the tool call id from content when present.
func ToolCallID(ev *StreamEventEnvelope) string {
	if ev == nil || ev.Content == nil {
		return ""
	}
	id, _ := ev.Content["toolCallId"].(string)
	return strings.TrimSpace(id)
}

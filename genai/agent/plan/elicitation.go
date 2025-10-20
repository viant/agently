package plan

// This file defines a light-weight representation of an “elicitation” prompt
// that is independent from the full MCP protocol types.  It is used by the
// CLI, unit-tests and higher-level services when interactive, schema-driven
// user input is required.

import (
	"strings"

	mcpproto "github.com/viant/mcp-protocol/schema"
)

// Elicitation describes a request to obtain a user-supplied payload that
// conforms to the supplied JSON Schema document (Schema field).  The struct
// embeds mcp-protocol’s ElicitRequestParams so that callers that already work
// with the full protocol can freely convert between the two without manual
// copying.
//
// Only a subset of the original fields is actually used by Agently today.  The
// additional Schema string is a convenience copy of RequestedSchema encoded as
// a single JSON document so that generic front-ends (CLI/stdio) do not have to
// reconstruct it from the sub-fields.
type Elicitation struct {
	mcpproto.ElicitRequestParams `json:",inline"`
	// CallbackURL is a server-relative URL that the UI can POST to
	// with a body {action, payload} to resolve this elicitation.
	// It is optional and, when present, preferred over generic
	// form submission so both LLM- and tool-initiated prompts share
	// a unified posting contract.
	CallbackURL string `json:"callbackURL,omitempty"`
}

// IsEmpty reports whether the elicitation is effectively empty (i.e. there is
// nothing to ask the user).  The heuristic is intentionally simple so that it
// does not require full JSON-Schema parsing.
func (e *Elicitation) IsEmpty() bool {
	if e == nil {
		return true
	}

	if strings.TrimSpace(e.Message) != "" {
		return false
	}
	// Fall back to inspecting embedded RequestedSchema.
	rs := e.RequestedSchema
	if len(rs.Properties) > 0 {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// ToolResult helpers used by Awaiters and callers
// ---------------------------------------------------------------------------

// ElicitResultAction defines the action selected by the user after the
// elicitation process finished.
type ElicitResultAction string

const (
	// ElicitResultActionAccept indicates that the user supplied a payload that
	// satisfies the schema and accepted the request.
	ElicitResultActionAccept ElicitResultAction = "accept"

	// ElicitResultActionDecline signals that the user declined to provide a
	// payload.
	ElicitResultActionDecline ElicitResultAction = "decline"
)

// ElicitResult represents the outcome of the elicitation prompt.
type ElicitResult struct {
	// Action is either "accept" or "decline".
	Action ElicitResultAction `json:"action"`

	// Payload is the user supplied map that conforms to the supplied schema
	// when Action == "accept". It is nil if the action is "decline".
	Payload map[string]any `json:"payload,omitempty"`

	// Reason optionally carries a human readable explanation when the user
	// declines an elicitation. It is empty for accept actions.
	Reason string `json:"reason,omitempty"`
}

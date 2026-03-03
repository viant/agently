package agents

import (
	"encoding/json"
	"strings"

	"github.com/viant/agently/genai/llm"
)

// ListItem is a directory entry describing an agent option for selection.
type ListItem struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name,omitempty"`
	Description      string                 `json:"description,omitempty"`
	Summary          string                 `json:"summary,omitempty"`
	Internal         bool                   `json:"internal,omitempty"`
	Tags             []string               `json:"tags,omitempty"`
	Priority         int                    `json:"priority,omitempty"`
	Capabilities     map[string]interface{} `json:"capabilities,omitempty"`
	Source           string                 `json:"source,omitempty"` // internal | external
	Responsibilities []string               `json:"responsibilities,omitempty"`
	InScope          []string               `json:"inScope,omitempty"`
	OutOfScope       []string               `json:"outOfScope,omitempty"`
}

// ListOutput defines the response payload for agents:list.
type ListOutput struct {
	Items []ListItem `json:"items"`
}

// RunInput defines the request payload for agents:run.
// Note: Conversation/turn/user identifiers are derived from context; they are
// intentionally not part of the input contract.
type RunInput struct {
	AgentID   string                 `json:"agentId"`
	Objective string                 `json:"objective"`
	Context   map[string]interface{} `json:"context,omitempty"`
	// ConversationID optionally overrides the conversation identifier when
	// not already provided by context.
	ConversationID string `json:"conversationId,omitempty"`
	// Streaming is an optional hint. Runtime policy/capabilities decide final behavior.
	Streaming *bool `json:"streaming,omitempty"`
	// ModelPreferences optionally hints how to select a model for this
	// run when the agent supports model preferences. When omitted, the
	// agent's configured model selection is used.
	ModelPreferences *llm.ModelPreferences `json:"modelPreferences,omitempty"`
	// ReasoningEffort optionally overrides agent-level reasoning effort
	// (e.g., low|medium|high) for this run when supported by the backend.
	ReasoningEffort *string `json:"reasoningEffort,omitempty"`
}

// UnmarshalJSON tolerates legacy/model-emitted string context values by
// accepting either:
// - context: { ... } (preferred)
// - context: "{\"k\":\"v\"}" (JSON object encoded as string)
// - context: "map[...]" (ignored as unusable; treated as empty context)
func (r *RunInput) UnmarshalJSON(data []byte) error {
	type Alias RunInput
	aux := &struct {
		Context json.RawMessage `json:"context,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	r.Context = nil
	raw := strings.TrimSpace(string(aux.Context))
	if raw == "" || raw == "null" {
		return nil
	}
	if strings.HasPrefix(raw, "{") {
		var m map[string]interface{}
		if err := json.Unmarshal(aux.Context, &m); err != nil {
			return err
		}
		r.Context = m
		return nil
	}
	if strings.HasPrefix(raw, "\"") {
		var s string
		if err := json.Unmarshal(aux.Context, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		if strings.HasPrefix(s, "{") {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(s), &m); err != nil {
				// Non-JSON string payloads are ignored to avoid hard tool failure.
				return nil
			}
			r.Context = m
		}
		return nil
	}
	// Unknown/non-object context shapes are ignored.
	return nil
}

// RunOutput defines the response payload for agents:run.
// Depending on routing (internal vs external), different handles will be set.
type RunOutput struct {
	Answer          string   `json:"answer"`
	Status          string   `json:"status,omitempty"`
	ConversationID  string   `json:"conversationId,omitempty"`
	MessageID       string   `json:"messageId,omitempty"`
	TaskID          string   `json:"taskId,omitempty"`
	ContextID       string   `json:"contextId,omitempty"`
	StreamSupported bool     `json:"streamSupported,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// MeOutput provides minimal execution context details.
type MeOutput struct {
	ConversationID string `json:"conversationId,omitempty"`
	AgentName      string `json:"agentName,omitempty"`
	Model          string `json:"model,omitempty"`
}

package agents

// ListItem is a directory entry describing an agent option for selection.
type ListItem struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name,omitempty"`
	Description      string                 `json:"description,omitempty"`
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
	// Streaming is an optional hint. Runtime policy/capabilities decide final behavior.
	Streaming *bool `json:"streaming,omitempty"`
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

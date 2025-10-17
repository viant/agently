package exec

// Input defines the run_agent request payload.
type Input struct {
	ConversationID       string `json:"conversationId,omitempty"`
	ParentConversationID string `json:"parentConversationId,omitempty"`
	MessageID            string `json:"messageId,omitempty"`

	AgentID   string                 `json:"agentId"`
	Objective string                 `json:"objective"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// Output defines the run_agent response payload.
type Output struct {
	Answer string `json:"answer"`
}

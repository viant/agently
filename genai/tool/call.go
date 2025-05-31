package tool

import "time"

// Call represents a single tool execution event/call in the business domain.
// It is intentionally persistence-agnostic so it can be reused by multiple
// storage adapters (SQL, NoSQL, event logs, etc.).
type Call struct {
    ID             int        `json:"id"`
    ConversationID string     `json:"conversationId,omitempty"`
    ToolName       string     `json:"toolName"`
    Arguments      *string    `json:"arguments,omitempty"`
    Result         *string    `json:"result,omitempty"`
    Succeeded      *bool      `json:"succeeded,omitempty"`
    ErrorMsg       *string    `json:"errorMsg,omitempty"`
    StartedAt      *time.Time `json:"startedAt,omitempty"`
    FinishedAt     *time.Time `json:"finishedAt,omitempty"`
}

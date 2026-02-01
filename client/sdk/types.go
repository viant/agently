package sdk

import conv "github.com/viant/agently/client/conversation"

// CreateConversationRequest mirrors POST /v1/api/conversations.
type CreateConversationRequest struct {
	Model      string `json:"model,omitempty"`
	Agent      string `json:"agent,omitempty"`
	Tools      string `json:"tools,omitempty"` // comma-separated
	Title      string `json:"title,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// CreateConversationResponse mirrors POST /v1/api/conversations response.
type CreateConversationResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	Model     string `json:"model,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Tools     string `json:"tools,omitempty"`
}

// ConversationSummary mirrors conversation list items.
type ConversationSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Summary    *string  `json:"summary,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
	Agent      string   `json:"agent,omitempty"`
	Model      string   `json:"model,omitempty"`
	Tools      []string `json:"tools,omitempty"`
}

// GetMessagesOptions configures transcript fetch.
type GetMessagesOptions struct {
	Since                   string
	IncludeModelCallPayload bool
	IncludeLinked           bool
}

// PostMessageRequest mirrors POST /v1/api/conversations/{id}/messages.
type PostMessageRequest struct {
	Content string                 `json:"content"`
	Agent   string                 `json:"agent,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Tools   []string               `json:"tools,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`

	ToolCallExposure *string  `json:"toolCallExposure,omitempty"`
	AutoSummarize    *bool    `json:"autoSummarize,omitempty"`
	AutoSelectTools  *bool    `json:"autoSelectTools,omitempty"`
	DisableChains    bool     `json:"disableChains,omitempty"`
	AllowedChains    []string `json:"allowedChains,omitempty"`

	Attachments []UploadedAttachment `json:"attachments,omitempty"`
}

// PostMessageResponse is a minimal wrapper for message id.
type PostMessageResponse struct {
	ID string `json:"id"`
}

// UploadedAttachment mirrors staged upload descriptor for message attachments.
type UploadedAttachment struct {
	Name          string `json:"name"`
	Size          int    `json:"size,omitempty"`
	StagingFolder string `json:"stagingFolder,omitempty"`
	URI           string `json:"uri"`
	Mime          string `json:"mime,omitempty"`
}

// UploadResponse mirrors /upload response.
type UploadResponse struct {
	Name          string `json:"name"`
	Size          int64  `json:"size,omitempty"`
	URI           string `json:"uri"`
	StagingFolder string `json:"stagingFolder,omitempty"`
}

// ToolRunRequest mirrors POST /v1/api/conversations/{id}/tools/run.
type ToolRunRequest struct {
	Service string                 `json:"service"`
	Method  string                 `json:"method"`
	Args    map[string]interface{} `json:"args,omitempty"`
}

// ToolRunResponse is a generic tool response payload.
type ToolRunResponse map[string]interface{}

// PayloadResponse wraps raw payload bytes and content type.
type PayloadResponse struct {
	ContentType string
	Body        []byte
}

// AuthProvider mirrors /v1/api/auth/providers.
type AuthProvider struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Mode  string `json:"mode"`
}

// LocalLoginRequest mirrors /v1/api/auth/local/login.
type LocalLoginRequest struct {
	Name string `json:"name"`
}

// OAuthInitiateResponse mirrors /v1/api/auth/oauth/initiate.
type OAuthInitiateResponse struct {
	AuthURL string `json:"authURL"`
}

// StreamEventEnvelope mirrors the server /events envelope.
type StreamEventEnvelope struct {
	Seq            uint64                 `json:"seq"`
	Time           string                 `json:"time"`
	ConversationID string                 `json:"conversationId"`
	Message        *conv.Message          `json:"message"`
	ContentType    string                 `json:"contentType,omitempty"`
	Content        map[string]interface{} `json:"content,omitempty"`
}

// ChatTurnUpdate carries merged output for a message.
type ChatTurnUpdate struct {
	MessageID string
	Text      string
	IsFinal   bool
}

// PollResponse is returned by long-poll events.
type PollResponse struct {
	Events []*StreamEventEnvelope `json:"events"`
	Since  string                 `json:"since,omitempty"`
}

// MessageBuffer accumulates assistant deltas and reconciles with final messages.
type MessageBuffer struct {
	ByMessageID map[string]string
}

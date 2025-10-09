package chat

import (
	agconv "github.com/viant/agently/pkg/agently/conversation"
	toolfeed "github.com/viant/agently/pkg/agently/tool"
)

// GetRequest defines inputs to fetch messages.
type GetRequest struct {
	ConversationID          string
	IncludeModelCallPayload bool
	SinceID                 string
	IncludeToolCall         bool
	ToolExtensions          []*toolfeed.FeedSpec
}

// GetResponse carries the rich conversation view for the given request.
type GetResponse struct{ Conversation *agconv.ConversationView }

// PostRequest defines inputs to submit a user message.
type PostRequest struct {
	Content     string                 `json:"content"`
	Agent       string                 `json:"agent,omitempty"`
	Model       string                 `json:"model,omitempty"`
	Tools       []string               `json:"tools,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Attachments []UploadedAttachment   `json:"attachments,omitempty"`
}

// UploadedAttachment mirrors Forge upload response structure.
type UploadedAttachment struct {
	Name          string `json:"name"`
	Size          int    `json:"size,omitempty"`
	StagingFolder string `json:"stagingFolder,omitempty"`
	URI           string `json:"uri"`
	Mime          string `json:"mime,omitempty"`
}

// CreateConversationRequest mirrors HTTP payload for POST /conversations.
type CreateConversationRequest struct {
	Model      string `json:"model"`
	Agent      string `json:"agent"`
	Tools      string `json:"tools"`
	Title      string `json:"title"`
	Visibility string `json:"visibility"`
}

// CreateConversationResponse echoes created entity details.
type CreateConversationResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	Model     string `json:"model,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Tools     string `json:"tools,omitempty"`
}

// ConversationSummary lists id + title only.
type ConversationSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

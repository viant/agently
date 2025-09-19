package chat

import (
	"context"

	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/tool"
	internal "github.com/viant/agently/internal/service/chat"
	"github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
)

type API interface {
	AttachManager(mgr *conversation.Manager, tp *tool.Policy, fp *policy.Policy)
	AttachApproval(svc approval.Service)
	Get(ctx context.Context, req GetRequest) (*GetResponse, error)
	PreflightPost(ctx context.Context, conversationID string, req PostRequest) error
	Post(ctx context.Context, conversationID string, req PostRequest) (string, error)
	Cancel(conversationID string) bool
	CancelTurn(turnID string) bool
	CreateConversation(ctx context.Context, in CreateConversationRequest) (*CreateConversationResponse, error)
	GetConversation(ctx context.Context, id string) (*ConversationSummary, error)
	ListConversations(ctx context.Context) ([]ConversationSummary, error)
	Approve(ctx context.Context, messageID, action, reason string) error
	Elicit(ctx context.Context, messageID, action string, payload map[string]interface{}) error
	GetPayload(ctx context.Context, id string) ([]byte, string, error)
}

// Public aliases to internal request/response types to avoid leaking internal package in user code.
type (
	GetRequest                 = internal.GetRequest
	GetResponse                = internal.GetResponse
	PostRequest                = internal.PostRequest
	CreateConversationRequest  = internal.CreateConversationRequest
	CreateConversationResponse = internal.CreateConversationResponse
	ConversationSummary        = internal.ConversationSummary
)

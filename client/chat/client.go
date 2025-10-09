package chat

import (
	"context"

	agconversation "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/conversation"
	core "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
)

type Client interface {
	AttachManager(mgr *conversation.Manager, tp *tool.Policy, fp *policy.Policy)
	AttachCore(core *core.Service)
	AttachApproval(svc approval.Service)
	Get(ctx context.Context, req GetRequest) (*GetResponse, error)
	PreflightPost(ctx context.Context, conversationID string, req PostRequest) error
	Post(ctx context.Context, conversationID string, req PostRequest) (string, error)
	Cancel(conversationID string) bool
	CancelTurn(turnID string) bool
	CreateConversation(ctx context.Context, in CreateConversationRequest) (*CreateConversationResponse, error)
	GetConversation(ctx context.Context, id string) (*ConversationSummary, error)
	ListConversations(ctx context.Context, input *agconversation.Input) ([]ConversationSummary, error)
	DeleteConversation(ctx context.Context, id string) error
	Approve(ctx context.Context, messageID, action, reason string) error
	Elicit(ctx context.Context, messageID, action string, payload map[string]interface{}) error
	GetPayload(ctx context.Context, id string) ([]byte, string, error)

	SetTurnStatus(ctx context.Context, turnID, status string, errorMessage ...string) error
	SetMessageStatus(ctx context.Context, messageID, status string) error
	SetLastAssistentMessageStatus(ctx context.Context, conversationID, status string) error

	// Generate exposes the low-level LLM core Generate bypassing agentic enrichment.
	Generate(ctx context.Context, input *core.GenerateInput) (*core.GenerateOutput, error)

	// Query executes an agentic turn synchronously with the provided input.
	// Returns the final content and metadata captured by the agent service.
	Query(ctx context.Context, input *QueryInput) (*QueryOutput, error)
}

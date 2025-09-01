package read

import (
	"context"
	"embed"
	"fmt"
	"reflect"
	"time"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"github.com/viant/xdatly/handler/response"
)

//go:embed sql/*.sql
var ConversationFS embed.FS

type ConversationInput struct {
	Summary         string                `parameter:",kind=query,in=summary" predicate:"contains,group=0,c,summary"`
	Id              string                `parameter:",kind=path,in=id" predicate:"in,group=0,c,id"`
	Title           string                `parameter:",kind=query,in=title" predicate:"contains,group=0,c,title"`
	AgentName       string                `parameter:",kind=query,in=agent_name" predicate:"contains,group=0,c,agent_name"`
	AgentID         string                `parameter:",kind=query,in=agent_id" predicate:"in,group=0,c,agent_id"`
	AgentConfigID   string                `parameter:",kind=query,in=agent_config_id" predicate:"in,group=0,c,agent_config_id"`
	Visibility      string                `parameter:",kind=query,in=visibility" predicate:"in,group=0,c,visibility"`
	TenantID        string                `parameter:",kind=query,in=tenant_id" predicate:"in,group=0,c,tenant_id"`
	CreatedByUserID string                `parameter:",kind=query,in=created_by_user_id" predicate:"in,group=0,c,created_by_user_id"`
	Archived        []int                 `parameter:",kind=query,in=archived" predicate:"in,group=0,c,archived"`
	Has             *ConversationInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ConversationInputHas struct {
	Summary         bool
	Id              bool
	Title           bool
	AgentName       bool
	AgentID         bool
	AgentConfigID   bool
	Visibility      bool
	TenantID        bool
	CreatedByUserID bool
	Archived        bool
}

type ConversationOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*ConversationView `parameter:",kind=output,in=view" view:"conversation,batch=10000,relationalConcurrency=1" sql:"uri=sql/conversation.sql"`
}

type ConversationView struct {
	Id                   string     `sqlx:"id"`
	Summary              *string    `sqlx:"summary"`
	AgentName            *string    `sqlx:"agent_name"`
	CreatedAt            *time.Time `sqlx:"created_at"`
	UpdatedAt            *time.Time `sqlx:"updated_at"`
	CreatedByUserID      *string    `sqlx:"created_by_user_id"`
	TenantID             *string    `sqlx:"tenant_id"`
	AgentID              *string    `sqlx:"agent_id"`
	AgentConfigID        *string    `sqlx:"agent_config_id"`
	DefaultModelProvider *string    `sqlx:"default_model_provider"`
	DefaultModel         *string    `sqlx:"default_model"`
	DefaultModelParams   *string    `sqlx:"default_model_params"`
	Title                *string    `sqlx:"title"`
	Metadata             *string    `sqlx:"metadata"`
	Visibility           *string    `sqlx:"visibility"`
	Archived             *int       `sqlx:"archived"`
	DeletedAt            *time.Time `sqlx:"deleted_at"`
	LastMessageAt        *time.Time `sqlx:"last_message_at"`
	LastTurnID           *string    `sqlx:"last_turn_id"`
	MessageCount         *int       `sqlx:"message_count"`
	TurnCount            *int       `sqlx:"turn_count"`
	RetentionTTLDays     *int       `sqlx:"retention_ttl_days"`
	ExpiresAt            *time.Time `sqlx:"expires_at"`
	LastActivity         *time.Time `sqlx:"last_activity"`
	UsageInputTokens     *int       `sqlx:"usage_input_tokens"`
	UsageOutputTokens    *int       `sqlx:"usage_output_tokens"`
	UsageEmbeddingTokens *int       `sqlx:"usage_embedding_tokens"`
}

var ConversationPathURI = "/v2/api/agently/conversation/{id}"
var ConversationBasePathURI = "/v2/api/agently/conversation"

func DefineConversationComponent(ctx context.Context, srv *datly.Service) error {
	compWithID, err := repository.NewComponent(
		contract.NewPath("GET", ConversationPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ConversationInput{}),
			reflect.TypeOf(ConversationOutput{}), &ConversationFS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create Conversation component (with id): %w", err)
	}
	if err := srv.AddComponent(ctx, compWithID); err != nil {
		return fmt.Errorf("failed to add Conversation component (with id): %w", err)
	}

	compBase, err := repository.NewComponent(
		contract.NewPath("GET", ConversationBasePathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ConversationInput{}),
			reflect.TypeOf(ConversationOutput{}), &ConversationFS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create Conversation component (base): %w", err)
	}
	if err := srv.AddComponent(ctx, compBase); err != nil {
		return fmt.Errorf("failed to add Conversation component (base): %w", err)
	}
	return nil
}

func (i *ConversationInput) EmbedFS() *embed.FS { return &ConversationFS }

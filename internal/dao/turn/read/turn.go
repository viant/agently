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
var FS embed.FS

type Input struct {
	ConversationID string     `parameter:",kind=path,in=conversationId" predicate:"in,group=0,t,conversation_id"`
	Id             string     `parameter:",kind=query,in=id" predicate:"in,group=0,t,id"`
	Ids            []string   `parameter:",kind=query,in=ids" predicate:"in,group=0,t,id"`
	Status         string     `parameter:",kind=query,in=status" predicate:"in,group=0,t,status"`
	Since          *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,t,created_at"`
	Has            *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	ConversationID bool
	Id             bool
	Ids            bool
	Status         bool
	Since          bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status,cacheable=false" json:",omitempty"`
	Data            []*TurnView      `parameter:",kind=output,in=view" view:"turn,batch=10000,relationalConcurrency=1" sql:"uri=sql/turn.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type TurnView struct {
	Id                    string     `sqlx:"id"`
	ConversationID        string     `sqlx:"conversation_id"`
	CreatedAt             *time.Time `sqlx:"created_at"`
	Status                string     `sqlx:"status"`
	StartedByMessageID    *string    `sqlx:"started_by_message_id"`
	RetryOf               *string    `sqlx:"retry_of"`
	AgentIDUsed           *string    `sqlx:"agent_id_used"`
	AgentConfigUsedID     *string    `sqlx:"agent_config_used_id"`
	ModelOverrideProvider *string    `sqlx:"model_override_provider"`
	ModelOverride         *string    `sqlx:"model_override"`
	ModelParamsOverride   *string    `sqlx:"model_params_override"`
}

var PathBase = "/v2/api/agently/turn"
var PathByConversation = "/v2/api/agently/conversation/{conversationId}/turn"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	// base path
	base, err := repository.NewComponent(
		contract.NewPath("GET", PathBase),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create turn base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add turn base: %w", err)
	}

	// by conversation path
	byConv, err := repository.NewComponent(
		contract.NewPath("GET", PathByConversation),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create turn by-conv component: %w", err)
	}
	if err := srv.AddComponent(ctx, byConv); err != nil {
		return fmt.Errorf("failed to add turn by-conv: %w", err)
	}

	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

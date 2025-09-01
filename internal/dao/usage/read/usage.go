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
	ConversationID string     `parameter:",kind=path,in=conversationId" predicate:"in,group=0,m,conversation_id"`
	Provider       string     `parameter:",kind=query,in=provider" predicate:"in,group=0,mc,provider"`
	Model          string     `parameter:",kind=query,in=model" predicate:"in,group=0,mc,model"`
	Since          *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,mc,started_at"`
	Has            *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	ConversationID bool
	Provider       bool
	Model          bool
	Since          bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*UsageView     `parameter:",kind=output,in=view" view:"usage,batch=10000,relationalConcurrency=1" sql:"uri=sql/usage.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type UsageView struct {
	ConversationID        string     `sqlx:"conversation_id"`
	Provider              string     `sqlx:"provider"`
	Model                 string     `sqlx:"model"`
	TotalPromptTokens     *int       `sqlx:"total_prompt_tokens"`
	TotalCompletionTokens *int       `sqlx:"total_completion_tokens"`
	TotalTokens           *int       `sqlx:"total_tokens"`
	TotalCost             *float64   `sqlx:"total_cost"`
	CallsCount            *int       `sqlx:"calls_count"`
	CachedCalls           *int       `sqlx:"cached_calls"`
	FirstCallAt           *time.Time `sqlx:"first_call_at"`
	LastCallAt            *time.Time `sqlx:"last_call_at"`
}

var PathBase = "/v2/api/agently/usage"
var PathByConversation = "/v2/api/agently/conversation/{conversationId}/usage"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	base, err := repository.NewComponent(
		contract.NewPath("GET", PathBase),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create usage base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add usage base: %w", err)
	}

	byConv, err := repository.NewComponent(
		contract.NewPath("GET", PathByConversation),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create usage by-conv component: %w", err)
	}
	if err := srv.AddComponent(ctx, byConv); err != nil {
		return fmt.Errorf("failed to add usage by-conv: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

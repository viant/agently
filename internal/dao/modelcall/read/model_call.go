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
	MessageID      string     `parameter:",kind=query,in=message_id" predicate:"in,group=0,mc,message_id"`
	MessageIDs     []string   `parameter:",kind=query,in=message_ids" predicate:"in,group=0,mc,message_id"`
	TurnID         string     `parameter:",kind=query,in=turn_id" predicate:"in,group=0,mc,turn_id"`
	Provider       string     `parameter:",kind=query,in=provider" predicate:"in,group=0,mc,provider"`
	Model          string     `parameter:",kind=query,in=model" predicate:"in,group=0,mc,model"`
	ModelKind      string     `parameter:",kind=query,in=model_kind" predicate:"in,group=0,mc,model_kind"`
	Since          *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,mc,started_at"`
	Has            *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	ConversationID bool
	MessageID      bool
	MessageIDs     bool
	TurnID         bool
	Provider       bool
	Model          bool
	ModelKind      bool
	Since          bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*ModelCallView `parameter:",kind=output,in=view" view:"model_call,batch=10000,relationalConcurrency=1" sql:"uri=sql/model_call.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type ModelCallView struct {
	MessageID         string     `sqlx:"message_id"`
	Status            string     `sqlx:"status"`
	TurnID            *string    `sqlx:"turn_id"`
	Provider          string     `sqlx:"provider"`
	Model             string     `sqlx:"model"`
	ModelKind         string     `sqlx:"model_kind"`
	PromptTokens      *int       `sqlx:"prompt_tokens"`
	CompletionTokens  *int       `sqlx:"completion_tokens"`
	TotalTokens       *int       `sqlx:"total_tokens"`
	FinishReason      *string    `sqlx:"finish_reason"`
	CacheHit          *int       `sqlx:"cache_hit"`
	CacheKey          *string    `sqlx:"cache_key"`
	StartedAt         *time.Time `sqlx:"started_at"`
	CompletedAt       *time.Time `sqlx:"completed_at"`
	LatencyMS         *int       `sqlx:"latency_ms"`
	Cost              *float64   `sqlx:"cost"`
	TraceID           *string    `sqlx:"trace_id"`
	SpanID            *string    `sqlx:"span_id"`
	RequestPayloadID  *string    `sqlx:"request_payload_id"`
	ResponsePayloadID *string    `sqlx:"response_payload_id"`
}

var PathBase = "/v2/api/agently/modelcall"
var PathByConversation = "/v2/api/agently/conversation/{conversationId}/modelcall"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	base, err := repository.NewComponent(
		contract.NewPath("GET", PathBase),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create modelcall base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add modelcall base: %w", err)
	}

	byConv, err := repository.NewComponent(
		contract.NewPath("GET", PathByConversation),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create modelcall by-conv component: %w", err)
	}
	if err := srv.AddComponent(ctx, byConv); err != nil {
		return fmt.Errorf("failed to add modelcall by-conv: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

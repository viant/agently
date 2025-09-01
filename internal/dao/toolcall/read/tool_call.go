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
	MessageID      string     `parameter:",kind=query,in=message_id" predicate:"in,group=0,tc,message_id"`
	MessageIDs     []string   `parameter:",kind=query,in=message_ids" predicate:"in,group=0,tc,message_id"`
	TurnID         string     `parameter:",kind=query,in=turn_id" predicate:"in,group=0,tc,turn_id"`
	OpID           string     `parameter:",kind=query,in=op_id" predicate:"in,group=0,tc,op_id"`
	ToolName       string     `parameter:",kind=query,in=tool_name" predicate:"in,group=0,tc,tool_name"`
	Status         string     `parameter:",kind=query,in=status" predicate:"in,group=0,tc,status"`
	Since          *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,tc,started_at"`
	Has            *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	ConversationID bool
	MessageID      bool
	MessageIDs     bool
	TurnID         bool
	OpID           bool
	ToolName       bool
	Status         bool
	Since          bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*ToolCallView  `parameter:",kind=output,in=view" view:"tool_call,batch=10000,relationalConcurrency=1" sql:"uri=sql/tool_call.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type ToolCallView struct {
	MessageID        string     `sqlx:"message_id"`
	TurnID           *string    `sqlx:"turn_id"`
	OpID             string     `sqlx:"op_id"`
	Attempt          int        `sqlx:"attempt"`
	ToolName         string     `sqlx:"tool_name"`
	ToolKind         string     `sqlx:"tool_kind"`
	CapabilityTags   *string    `sqlx:"capability_tags"`
	ResourceURIs     *string    `sqlx:"resource_uris"`
	Status           string     `sqlx:"status"`
	RequestSnapshot  *string    `sqlx:"request_snapshot"`
	RequestHash      *string    `sqlx:"request_hash"`
	ResponseSnapshot *string    `sqlx:"response_snapshot"`
	ErrorCode        *string    `sqlx:"error_code"`
	ErrorMessage     *string    `sqlx:"error_message"`
	Retriable        *int       `sqlx:"retriable"`
	StartedAt        *time.Time `sqlx:"started_at"`
	CompletedAt      *time.Time `sqlx:"completed_at"`
	LatencyMS        *int       `sqlx:"latency_ms"`
	Cost             *float64   `sqlx:"cost"`
	TraceID          *string    `sqlx:"trace_id"`
	SpanID           *string    `sqlx:"span_id"`
}

var PathBase = "/v2/api/agently/toolcall"
var PathByConversation = "/v2/api/agently/conversation/{conversationId}/toolcall"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	base, err := repository.NewComponent(
		contract.NewPath("GET", PathBase),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create toolcall base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add toolcall base: %w", err)
	}

	byConv, err := repository.NewComponent(
		contract.NewPath("GET", PathByConversation),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create toolcall by-conv component: %w", err)
	}
	if err := srv.AddComponent(ctx, byConv); err != nil {
		return fmt.Errorf("failed to add toolcall by-conv: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

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
	Id             string     `parameter:",kind=query,in=id" predicate:"in,group=0,m,id"`
	Ids            []string   `parameter:",kind=query,in=ids" predicate:"in,group=0,m,id"`
	TurnID         string     `parameter:",kind=query,in=turn_id" predicate:"in,group=0,m,turn_id"`
	Roles          []string   `parameter:",kind=query,in=roles" predicate:"in,group=0,m,roles"`
	Type           string     `parameter:",kind=query,in=type" predicate:"in,group=0,m,type"`
	Interim        []int      `parameter:",kind=query,in=interim" predicate:"in,group=0,m,interim"`
	ElicitationID  string     `parameter:",kind=query,in=elicitation_id" predicate:"in,group=0,m,elicitation_id"`
	Since          *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,m,created_at"`
	Has            *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	ConversationID bool
	Id             bool
	Ids            bool
	TurnID         bool
	Roles          bool
	Type           bool
	Interim        bool
	ElicitationID  bool
	Since          bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*MessageView   `parameter:",kind=output,in=view" view:"message,batch=10000,relationalConcurrency=1" sql:"uri=sql/message.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type MessageView struct {
	Id               string     `sqlx:"id"`
	ConversationID   string     `sqlx:"conversation_id"`
	TurnID           *string    `sqlx:"turn_id"`
	Sequence         *int       `sqlx:"sequence"`
	CreatedAt        *time.Time `sqlx:"created_at"`
	Role             string     `sqlx:"role"`
	Type             string     `sqlx:"type"`
	Content          string     `sqlx:"content"`
	ContextSummary   *string    `sqlx:"context_summary"`
	Tags             *string    `sqlx:"tags"`
	Interim          *int       `sqlx:"interim"`
	ElicitationID    *string    `sqlx:"elicitation_id"`
	ParentMessageID  *string    `sqlx:"parent_message_id"`
	ModelCallPresent *int       `sqlx:"model_call_present"`
	ToolCallPresent  *int       `sqlx:"tool_call_present"`
	SupersededBy     *string    `sqlx:"superseded_by"`
	ToolName         *string    `sqlx:"tool_name"`

	ToolCall  *ToolCallView  `sqlx:"-" on:"Id:id=MessageID:message_id" view:",table=tool_calls" sql:"uri=sql/tool_call.sql"`
	ModelCall *ModelCallView `sqlx:"-" on:"Id:id=MessageID:message_id" view:",table=model_calls" sql:"uri=sql/model_call.sql"`
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

type ModelCallView struct {
	MessageID         string     `sqlx:"message_id"`
	Status            string     `sqlx:"status"`
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

var PathBase = "/v2/api/agently/message"
var PathByConversation = "/v2/api/agently/conversation/{conversationId}/message"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	base, err := repository.NewComponent(
		contract.NewPath("GET", PathBase),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create message base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add message base: %w", err)
	}

	byConv, err := repository.NewComponent(
		contract.NewPath("GET", PathByConversation),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create message by-conv component: %w", err)
	}
	if err := srv.AddComponent(ctx, byConv); err != nil {
		return fmt.Errorf("failed to add message by-conv: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

package read

import (
	"context"
	"embed"
	"fmt"
	"reflect"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/types/core"
	"github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("toolcall", "ByOpInput", reflect.TypeOf(ByOpInput{}), checksum.GeneratedTime)
	core.RegisterType("toolcall", "ByOpOutput", reflect.TypeOf(ByOpOutput{}), checksum.GeneratedTime)
}

//go:embed sql/*.sql
var FS embed.FS

type ByOpInput struct {
	ConversationId string        `parameter:",kind=query,in=conversationId" predicate:"expr,group=0,m.conversation_id = ?"`
	OpId           string        `parameter:",kind=path,in=opId" predicate:"equal,group=1,t,op_id"`
	Has            *ByOpInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ByOpInputHas struct {
	ConversationId bool
	OpId           bool
}

type ByOpOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*ToolCallRow   `parameter:",kind=output,in=view" view:",batch=10000,relationalConcurrency=1" sql:"uri=sql/tool_call_by_op.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type ToolCallRow struct {
	MessageId         string  `sqlx:"message_id"`
	TurnId            *string `sqlx:"turn_id"`
	OpId              string  `sqlx:"op_id"`
	TraceId           *string `sqlx:"trace_id"`
	ResponsePayloadId *string `sqlx:"response_payload_id"`
}

var PathURI = "/v1/api/agently/toolcall/by-op/{opId}"

func DefineComponent(ctx context.Context, srv *datly.Service) (contract.Path, error) {
	comp, err := repository.NewComponent(
		contract.NewPath("GET", PathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ByOpInput{}),
			reflect.TypeOf(ByOpOutput{}), &FS, view.WithConnectorRef("agently")))
	if err != nil {
		return contract.Path{}, fmt.Errorf("failed to create toolcall/by-op component: %w", err)
	}
	if err := srv.AddComponent(ctx, comp); err != nil {
		return contract.Path{}, fmt.Errorf("failed to add toolcall/by-op component: %w", err)
	}
	return comp.Path, nil
}

func (i *ByOpInput) EmbedFS() *embed.FS { return &FS }

package read

import (
	"context"
	"embed"
	"fmt"
	"reflect"

	"github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/types/core"
	"github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	core.RegisterType("turn", "QueuedListInput", reflect.TypeOf(QueuedListInput{}), checksum.GeneratedTime)
	core.RegisterType("turn", "QueuedListOutput", reflect.TypeOf(QueuedListOutput{}), checksum.GeneratedTime)
}

type QueuedListInput struct {
	ConversationID string              `parameter:",kind=query,in=conversationId" predicate:"equal,group=0,t,conversation_id"`
	Has            *QueuedListInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type QueuedListInputHas struct {
	ConversationID bool
}

type QueuedTurnView struct {
	ID       string `sqlx:"id"`
	QueueSeq *int64 `sqlx:"queue_seq"`
}

type QueuedListOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*QueuedTurnView `parameter:",kind=output,in=view" view:"turnQueuedList,batch=1,relationalConcurrency=1" sql:"uri=conversation/turn_queued_list.sql"`
	Metrics         response.Metrics  `parameter:",kind=output,in=metrics"`
}

// QueuedListPathURI is intended for internal use via datly.Operate.
var QueuedListPathURI = "/v1/api/agently/turn/queuedList"

func DefineQueuedListComponent(ctx context.Context, srv *datly.Service) error {
	component, err := repository.NewComponent(
		contract.NewPath("GET", QueuedListPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(QueuedListInput{}),
			reflect.TypeOf(QueuedListOutput{}),
			&conversation.ConversationFS,
			view.WithConnectorRef("agently"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create Turn component: %w", err)
	}
	if err := srv.AddComponent(ctx, component); err != nil {
		return fmt.Errorf("failed to add Turn component: %w", err)
	}
	return nil
}

func (i *QueuedListInput) EmbedFS() *embed.FS { return &conversation.ConversationFS }

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
	core.RegisterType("turn", "QueuedCountInput", reflect.TypeOf(QueuedCountInput{}), checksum.GeneratedTime)
	core.RegisterType("turn", "QueuedCountOutput", reflect.TypeOf(QueuedCountOutput{}), checksum.GeneratedTime)
}

type QueuedCountInput struct {
	ConversationID string               `parameter:",kind=query,in=conversationId" predicate:"equal,group=0,t,conversation_id"`
	Has            *QueuedCountInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type QueuedCountInputHas struct {
	ConversationID bool
}

type QueuedCountOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*QueuedCountView `parameter:",kind=output,in=view" view:"turnQueuedCount,batch=1,relationalConcurrency=1" sql:"uri=conversation/turn_queued_count.sql"`
	Metrics         response.Metrics   `parameter:",kind=output,in=metrics"`
}

type QueuedCountView struct {
	QueuedCount int `sqlx:"queued_count"`
}

// QueuedCountPathURI is intended for internal use via datly.Operate.
var QueuedCountPathURI = "/v1/api/agently/turn/queuedCount"

func DefineQueuedCountComponent(ctx context.Context, srv *datly.Service) error {
	component, err := repository.NewComponent(
		contract.NewPath("GET", QueuedCountPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(QueuedCountInput{}),
			reflect.TypeOf(QueuedCountOutput{}),
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

func (i *QueuedCountInput) EmbedFS() *embed.FS {
	return &conversation.ConversationFS
}

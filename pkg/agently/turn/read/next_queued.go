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
	core.RegisterType("turn", "NextQueuedInput", reflect.TypeOf(NextQueuedInput{}), checksum.GeneratedTime)
	core.RegisterType("turn", "NextQueuedOutput", reflect.TypeOf(NextQueuedOutput{}), checksum.GeneratedTime)
}

type NextQueuedInput struct {
	ConversationID string              `parameter:",kind=query,in=conversationId" predicate:"equal,group=0,t,conversation_id"`
	Has            *NextQueuedInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type NextQueuedInputHas struct {
	ConversationID bool
}

type NextQueuedOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*conversation.TranscriptView `parameter:",kind=output,in=view" view:"conversation,batch=1,relationalConcurrency=1" sql:"uri=conversation/turn_next_queued.sql"`
	Metrics         response.Metrics               `parameter:",kind=output,in=metrics"`
}

// NextQueuedPathURI is intended for internal use via datly.Operate.
var NextQueuedPathURI = "/v1/api/agently/turn/nextQueued"

func DefineNextQueuedComponent(ctx context.Context, srv *datly.Service) error {
	component, err := repository.NewComponent(
		contract.NewPath("GET", NextQueuedPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(NextQueuedInput{}),
			reflect.TypeOf(NextQueuedOutput{}),
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

func (i *NextQueuedInput) EmbedFS() *embed.FS {
	return &conversation.ConversationFS
}

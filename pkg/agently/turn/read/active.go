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
	core.RegisterType("turn", "ActiveTurnInput", reflect.TypeOf(ActiveTurnInput{}), checksum.GeneratedTime)
	core.RegisterType("turn", "ActiveTurnOutput", reflect.TypeOf(ActiveTurnOutput{}), checksum.GeneratedTime)
}

type ActiveTurnInput struct {
	ConversationID string              `parameter:",kind=query,in=conversationId" predicate:"equal,group=0,t,conversation_id"`
	Has            *ActiveTurnInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ActiveTurnInputHas struct {
	ConversationID bool
}

type ActiveTurnOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*conversation.TranscriptView `parameter:",kind=output,in=view" view:"conversation,batch=1,relationalConcurrency=1" sql:"uri=conversation/turn_active.sql"`
	Metrics         response.Metrics               `parameter:",kind=output,in=metrics"`
}

// ActiveTurnPathURI is intended for internal use via datly.Operate.
var ActiveTurnPathURI = "/v1/api/agently/turn/active"

func DefineActiveTurnComponent(ctx context.Context, srv *datly.Service) error {
	component, err := repository.NewComponent(
		contract.NewPath("GET", ActiveTurnPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ActiveTurnInput{}),
			reflect.TypeOf(ActiveTurnOutput{}),
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

func (i *ActiveTurnInput) EmbedFS() *embed.FS {
	return &conversation.ConversationFS
}

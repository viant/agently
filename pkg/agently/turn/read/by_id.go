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
	core.RegisterType("turn", "TurnByIDInput", reflect.TypeOf(TurnByIDInput{}), checksum.GeneratedTime)
	core.RegisterType("turn", "TurnByIDOutput", reflect.TypeOf(TurnByIDOutput{}), checksum.GeneratedTime)
}

type TurnByIDInput struct {
	ID             string            `parameter:",kind=query,in=id" predicate:"equal,group=0,t,id"`
	ConversationID string            `parameter:",kind=query,in=conversationId" predicate:"equal,group=0,t,conversation_id"`
	Has            *TurnByIDInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type TurnByIDInputHas struct {
	ID             bool
	ConversationID bool
}

type TurnByIDOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*conversation.TranscriptView `parameter:",kind=output,in=view" view:"conversation,batch=1,relationalConcurrency=1" sql:"uri=conversation/turn_by_id.sql"`
	Metrics         response.Metrics               `parameter:",kind=output,in=metrics"`
}

// TurnByIDPathURI is intended for internal use via datly.Operate.
var TurnByIDPathURI = "/v1/api/agently/turn/byId"

func DefineTurnByIDComponent(ctx context.Context, srv *datly.Service) error {
	component, err := repository.NewComponent(
		contract.NewPath("GET", TurnByIDPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(TurnByIDInput{}),
			reflect.TypeOf(TurnByIDOutput{}),
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

func (i *TurnByIDInput) EmbedFS() *embed.FS { return &conversation.ConversationFS }

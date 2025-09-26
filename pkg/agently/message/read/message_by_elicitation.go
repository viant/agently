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
	core.RegisterType("message", "MessageByElicitationInput", reflect.TypeOf(MessageByElicitationInput{}), checksum.GeneratedTime)
	core.RegisterType("message", "MessageByElicitationOutput", reflect.TypeOf(MessageByElicitationOutput{}), checksum.GeneratedTime)
}

type MessageByElicitationInput struct {
	ConversationId string `parameter:",kind=path,in=convId"`
	ElicitationId  string `parameter:",kind=path,in=elicId"`
}

type MessageByElicitationOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*conversation.MessageView `parameter:",kind=output,in=view" view:"conversation,batch=10000,relationalConcurrency=1" sql:"uri=conversation/message_elicitation.sql"`
	Metrics         response.Metrics            `parameter:",kind=output,in=metrics"`
}

var MessageByElicitationPathURI = "/v1/api/agently/message/elicitation/{convId}/{elicId}"

func DefineMessageByElicitationComponent(ctx context.Context, srv *datly.Service) error {
	comp, err := repository.NewComponent(
		contract.NewPath("GET", MessageByElicitationPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(MessageByElicitationInput{}),
			reflect.TypeOf(MessageByElicitationOutput{}), &conversation.ConversationFS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create MessageByElicitation component: %w", err)
	}
	if err := srv.AddComponent(ctx, comp); err != nil {
		return fmt.Errorf("failed to add MessageByElicitation component: %w", err)
	}
	return nil
}

func (i *MessageByElicitationInput) EmbedFS() *embed.FS { return &conversation.ConversationFS }

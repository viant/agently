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
	core.RegisterType("message", "MessageInput", reflect.TypeOf(MessageInput{}), checksum.GeneratedTime)
	core.RegisterType("message", "MessageOutput", reflect.TypeOf(MessageOutput{}), checksum.GeneratedTime)
}

type MessageInput struct {
	Id              string           `parameter:",kind=path,in=id"`
	IncludeModelCal bool             `parameter:",kind=query,in=includeModelCall" predicate:"expr,group=2,?" value:"false"`
	IncludeToolCall bool             `parameter:",kind=query,in=includeToolCall" predicate:"expr,group=3,?" value:"false"`
	Has             *MessageInputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type MessageInputHas struct {
	Id                bool
	Since             bool
	IncludeTranscript bool
	IncludeModelCal   bool
	IncludeToolCall   bool
}

type MessageOutput struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*conversation.MessageView `parameter:",kind=output,in=view" view:"conversation,batch=10000,relationalConcurrency=1" sql:"uri=conversation/message.sql"`
	Metrics         response.Metrics            `parameter:",kind=output,in=metrics"`
}

var MessagePathURI = "/v1/api/agently/message/{id}"

func DefineMessageComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", MessagePathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(MessageInput{}),
			reflect.TypeOf(MessageOutput{}), &conversation.ConversationFS, view.WithConnectorRef("agently")))

	if err != nil {
		return fmt.Errorf("failed to create Conversation component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add Conversation component: %w", err)
	}
	return nil
}

func (i *MessageInput) EmbedFS() *embed.FS {
	return &conversation.ConversationFS
}

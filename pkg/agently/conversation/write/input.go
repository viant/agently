package write

import (
	"embed"
)

//go:embed sql/*.sql
var ConversationPostFS embed.FS

type Input struct {
	Conversations []*Conversation `parameter:",kind=body,in=data"`

	CurConversationsId *struct{ Values []string } `parameter:",kind=param,in=Conversations,dataType=conversation.Conversations" codec:"structql,uri=sql/cur_conversations_id.sql"`

	CurConversation []*Conversation `parameter:",kind=view,in=CurConversation" view:"CurConversation" sql:"uri=sql/cur_conversation.sql"`

	CurConversationById IndexedConversation
}

func (i *Input) EmbedFS() (fs *embed.FS) {
	return &ConversationPostFS
}

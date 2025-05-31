package post

import (
	"embed"
)

//go:embed conversation/*.sql
var ConversationPostFS embed.FS

type Input struct {
	Conversations []*Conversation `parameter:",kind=body,in=data"`

	CurConversationsId *struct{ Values []string } `parameter:",kind=param,in=Conversations,dataType=conversation.Conversations" codec:"structql,uri=conversation/cur_conversations_id.sql"`

	CurConversationsMessageId *struct{ Values []string } `parameter:",kind=param,in=Conversations,dataType=conversation.Conversations" codec:"structql,uri=conversation/cur_conversations_message_id.sql"`

	CurMessage []*Message `parameter:",kind=view,in=CurMessage" view:"CurMessage" sql:"uri=conversation/cur_message.sql"`

	CurConversation []*Conversation `parameter:",kind=view,in=CurConversation" view:"CurConversation" sql:"uri=conversation/cur_conversation.sql"`

	CurMessageById      IndexedMessage
	CurConversationById IndexedConversation
}

func (i *Input) EmbedFS() (fs *embed.FS) {
	return &ConversationPostFS
}

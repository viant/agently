package message

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Messages []*Message `parameter:",kind=body,in=data"`

	CurMessagesId *struct{ Values []string } `parameter:",kind=param,in=Messages,dataType=message/write.Messages" codec:"structql,uri=sql/cur_messages_id.sql"`

	CurMessage []*Message `parameter:",kind=view,in=CurMessage" view:"CurMessage" sql:"uri=sql/cur_message.sql"`

	CurMessageById map[string]*Message
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }

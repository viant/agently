package post

import (
	"context"
	"github.com/viant/xdatly/handler"
	"time"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, output *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}

	for _, conversation := range i.Conversations {
		if conversation.Has == nil {
			conversation.Has = &ConversationHas{}
		}
		now := time.Now()
		conversation.SetLastActivity(now)
	}
	i.indexSlice()
	return nil
}

func (i *Input) indexSlice() {
	i.CurMessageById = MessageSlice(i.CurMessage).IndexById()
	i.CurConversationById = ConversationSlice(i.CurConversation).IndexById()
}

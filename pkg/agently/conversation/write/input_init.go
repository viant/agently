package write

import (
	"context"
	"time"

	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, output *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}

	i.indexSlice()
	for _, conversation := range i.Conversations {
		if conversation.Has == nil {
			conversation.Has = &ConversationHas{}
		}
		now := time.Now()
		if _, ok := i.CurConversationById[conversation.Id]; !ok {
			conversation.SetCreatedAt(now)
		}
		conversation.SetLastActivity(now)
	}
	return nil
}

func (i *Input) indexSlice() {
	i.CurConversationById = ConversationSlice(i.CurConversation).IndexById()
}

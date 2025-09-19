//go:build dao_sql

package factory

import (
	"context"

	convsql "github.com/viant/agently/internal/dao/conversation/impl/sql"
	msgsql "github.com/viant/agently/internal/dao/message/impl/sql"
	mcpsql "github.com/viant/agently/internal/dao/modelcall/impl/sql"
	plsql "github.com/viant/agently/internal/dao/payload/impl/sql"
	tcsql "github.com/viant/agently/internal/dao/toolcall/impl/sql"
	turnsql "github.com/viant/agently/internal/dao/turn/impl/sql"
	"github.com/viant/datly"
)

func newSQL(ctx context.Context, dao *datly.Service) (*API, error) {
	if dao == nil {
		return nil, nil
	}
	_ = msgsql.Register(ctx, dao)
	_ = mcpsql.Register(ctx, dao)
	_ = tcsql.Register(ctx, dao)
	_ = plsql.Register(ctx, dao)
	_ = turnsql.Register(ctx, dao)
	_ = convsql.Register(ctx, dao)
	return &API{
		Conversation: convsql.New(ctx, dao),
		Message:      msgsql.New(ctx, dao),
		ModelCall:    mcpsql.New(ctx, dao),
		ToolCall:     tcsql.New(ctx, dao),
		Payload:      plsql.New(ctx, dao),
		Turn:         turnsql.New(ctx, dao),
	}, nil
}

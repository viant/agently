package factory

import (
	"context"

	convmem "github.com/viant/agently/internal/dao/conversation/impl/memory"
	convsql "github.com/viant/agently/internal/dao/conversation/impl/sql"
	msgmem "github.com/viant/agently/internal/dao/message/impl/memory"
	msgsql "github.com/viant/agently/internal/dao/message/impl/sql"
	mcpmem "github.com/viant/agently/internal/dao/modelcall/impl/memory"
	mcpsql "github.com/viant/agently/internal/dao/modelcall/impl/sql"
	plmem "github.com/viant/agently/internal/dao/payload/impl/memory"
	plsql "github.com/viant/agently/internal/dao/payload/impl/sql"
	tcmem "github.com/viant/agently/internal/dao/toolcall/impl/memory"
	tcsql "github.com/viant/agently/internal/dao/toolcall/impl/sql"
	turnmem "github.com/viant/agently/internal/dao/turn/impl/memory"
	turnsql "github.com/viant/agently/internal/dao/turn/impl/sql"
	usagemem "github.com/viant/agently/internal/dao/usage/impl/memory"
	usagesql "github.com/viant/agently/internal/dao/usage/impl/sql"
	"github.com/viant/datly"
)

func newMemory(_ context.Context) (*API, error) {
	return &API{
		Conversation: convmem.New(),
		Message:      msgmem.New(),
		ModelCall:    mcpmem.New(),
		ToolCall:     tcmem.New(),
		Payload:      plmem.New(),
		Turn:         turnmem.New(),
		Usage:        usagemem.New(),
	}, nil
}

func newSQL(ctx context.Context, dao *datly.Service) (*API, error) {
	if dao == nil {
		return nil, nil
	}
	err := msgsql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	err = mcpsql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	err = tcsql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	err = plsql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	err = turnsql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	err = usagesql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	err = convsql.Register(ctx, dao)
	if err != nil {
		return nil, err
	}
	return &API{
		Conversation: convsql.New2(ctx, dao),
		Message:      msgsql.New(ctx, dao),
		ModelCall:    mcpsql.New(ctx, dao),
		ToolCall:     tcsql.New(ctx, dao),
		Payload:      plsql.New(ctx, dao),
		Turn:         turnsql.New(ctx, dao),
		Usage:        usagesql.New(ctx, dao),
	}, nil
}

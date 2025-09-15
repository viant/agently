package factory

import (
	"context"

	convdao "github.com/viant/agently/internal/dao/conversation"
	msgdao "github.com/viant/agently/internal/dao/message"
	mcdao "github.com/viant/agently/internal/dao/modelcall"
	pldao "github.com/viant/agently/internal/dao/payload"
	tcdao "github.com/viant/agently/internal/dao/toolcall"
	turndao "github.com/viant/agently/internal/dao/turn"
	usagedao "github.com/viant/agently/internal/dao/usage"
	"github.com/viant/datly"
)

// DAOKind chooses between memory and SQL implementations.
type DAOKind string

const (
	DAOInMemory DAOKind = "memory"
	DAOSQL      DAOKind = "sql"
)

// API groups API APIs produced by New.
type API struct {
	Conversation  convdao.API
	Message       msgdao.API
	ModelCall     mcdao.API
	ToolCall      tcdao.API
	Payload       pldao.API
	Turn          turndao.API
	Usage         usagedao.API
	Conversation2 convdao.APIV2
}

// New returns API APIs for the requested kind.
// When kind is DAOSQL, dao must be provided; when built without dao_sql tag
// it will return (nil, nil).
func New(ctx context.Context, kind DAOKind, dao *datly.Service) (*API, error) {
	switch kind {
	case DAOInMemory:
		return newMemory(ctx)
	case DAOSQL:
		return newSQL(ctx, dao)
	default:
		return nil, nil
	}
}

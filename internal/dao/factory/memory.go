package factory

import (
	"context"

	convmem "github.com/viant/agently/internal/dao/conversation/impl/memory"
	msgmem "github.com/viant/agently/internal/dao/message/impl/memory"
	mcpmem "github.com/viant/agently/internal/dao/modelcall/impl/memory"
	plmem "github.com/viant/agently/internal/dao/payload/impl/memory"
	tcmem "github.com/viant/agently/internal/dao/toolcall/impl/memory"
	turnmem "github.com/viant/agently/internal/dao/turn/impl/memory"
	usagemem "github.com/viant/agently/internal/dao/usage/impl/memory"
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

// Stub to satisfy common factory when SQL build tag is not set.
func newSQL(_ context.Context, _ interface{}) (*API, error) { return nil, nil }

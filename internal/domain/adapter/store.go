package adapter

import (
	convdao "github.com/viant/agently/internal/dao/conversation"
	daofactory "github.com/viant/agently/internal/dao/factory"
	msgdao "github.com/viant/agently/internal/dao/message"
	mcdao "github.com/viant/agently/internal/dao/modelcall"
	pldao "github.com/viant/agently/internal/dao/payload"
	tcdao "github.com/viant/agently/internal/dao/toolcall"
	turndao "github.com/viant/agently/internal/dao/turn"
	domain "github.com/viant/agently/internal/domain"
	convsvc "github.com/viant/agently/internal/domain/adapter/conversation"
	msgsvc "github.com/viant/agently/internal/domain/adapter/message"
	opsvc "github.com/viant/agently/internal/domain/adapter/operations"
	payloadsvc "github.com/viant/agently/internal/domain/adapter/payload"
	turnsvc "github.com/viant/agently/internal/domain/adapter/turn"
)

// Store adapts DAO repositories to the domain.Store interface.
type Store struct {
	conv  convdao.API
	msg   msgdao.API
	turn  turndao.API
	model mcdao.API
	tool  tcdao.API
	pl    pldao.API

	messages   domain.Messages
	operations domain.Operations
	payloads   domain.Payloads
	turns      domain.Turns
	convs      domain.Conversations
}

// New constructs a Store with supplied DAO backends.
func New(conv convdao.API, msg msgdao.API, turn turndao.API, model mcdao.API, tool tcdao.API, pl pldao.API) *Store {
	s := &Store{conv: conv, msg: msg, turn: turn, model: model, tool: tool, pl: pl}
	s.messages = msgsvc.New(&daofactory.API{Message: msg, Payload: pl, ModelCall: model, ToolCall: tool})
	s.payloads = payloadsvc.New(pl)
	s.turns = turnsvc.New(turn)
	// Conversations now use a dedicated adapter backed by DAO read/write models.
	s.convs = convsvc.New(conv)
	s.operations = opsvc.New(&daofactory.API{Conversation: conv, Message: msg, ModelCall: model, ToolCall: tool, Payload: pl, Turn: turn})
	return s
}

// Ensure interface conformance
var _ domain.Store = (*Store)(nil)

func (s *Store) Conversations() domain.Conversations { return s.convs }
func (s *Store) Messages() domain.Messages           { return s.messages }
func (s *Store) Turns() domain.Turns                 { return s.turns }
func (s *Store) Operations() domain.Operations       { return s.operations }
func (s *Store) Payloads() domain.Payloads           { return s.payloads }

// ----------------------- Conversations -----------------------

// conversationsAdapter removed; replaced with dedicated adapter/service in adapter/conversation.

// ResponsePayload adapter moved to internal/domain/adapter/payload.

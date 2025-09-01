package adapter

import (
	convdao "github.com/viant/agently/internal/dao/conversation"
	daofactory "github.com/viant/agently/internal/dao/factory"
	msgdao "github.com/viant/agently/internal/dao/message"
	mcdao "github.com/viant/agently/internal/dao/modelcall"
	pldao "github.com/viant/agently/internal/dao/payload"
	tcdao "github.com/viant/agently/internal/dao/toolcall"
	turndao "github.com/viant/agently/internal/dao/turn"
	usagedao "github.com/viant/agently/internal/dao/usage"
	d "github.com/viant/agently/internal/domain"
	convsvc "github.com/viant/agently/internal/domain/adapter/conversation"
	msgsvc "github.com/viant/agently/internal/domain/adapter/message"
	opsvc "github.com/viant/agently/internal/domain/adapter/operations"
	payloadsvc "github.com/viant/agently/internal/domain/adapter/payload"
	turnsvc "github.com/viant/agently/internal/domain/adapter/turn"
	usagesvc "github.com/viant/agently/internal/domain/adapter/usage"
)

// Store adapts DAO repositories to the domain.Store interface.
type Store struct {
	conv  convdao.API
	msg   msgdao.API
	turn  turndao.API
	model mcdao.API
	tool  tcdao.API
	pl    pldao.API
	use   usagedao.API

	messages   d.Messages
	operations d.Operations
	payloads   d.Payloads
	usage      d.Usage
	turns      d.Turns
	convs      d.Conversations
}

// New constructs a Store with supplied DAO backends.
func New(conv convdao.API, msg msgdao.API, turn turndao.API, model mcdao.API, tool tcdao.API, pl pldao.API, use usagedao.API) *Store {
	s := &Store{conv: conv, msg: msg, turn: turn, model: model, tool: tool, pl: pl, use: use}
	s.messages = msgsvc.New(&daofactory.API{Message: msg, Payload: pl, ModelCall: model, ToolCall: tool, Usage: use})
	s.operations = opsvc.New(&daofactory.API{ModelCall: model, ToolCall: tool, Payload: pl})
	s.payloads = payloadsvc.New(pl)
	s.usage = usagesvc.New(use)
	s.turns = turnsvc.New(turn)
	// Conversations now use a dedicated adapter backed by DAO read/write models.
	s.convs = convsvc.New(conv, use)
	return s
}

// Ensure interface conformance
var _ d.Store = (*Store)(nil)

func (s *Store) Conversations() d.Conversations { return s.convs }
func (s *Store) Messages() d.Messages           { return s.messages }
func (s *Store) Turns() d.Turns                 { return s.turns }
func (s *Store) Operations() d.Operations       { return s.operations }
func (s *Store) Payloads() d.Payloads           { return s.payloads }
func (s *Store) Usage() d.Usage                 { return s.usage }

// ----------------------- Conversations -----------------------

// conversationsAdapter removed; replaced with dedicated adapter/service in adapter/conversation.

// Payload adapter moved to internal/domain/adapter/payload.

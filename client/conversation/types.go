package conversation

import (
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgw "github.com/viant/agently/pkg/agently/message"
	mcall "github.com/viant/agently/pkg/agently/modelcall"
	payloadw "github.com/viant/agently/pkg/agently/payload"
	payloadread "github.com/viant/agently/pkg/agently/payload/read"
	toolcall "github.com/viant/agently/pkg/agently/toolcall"
	turnw "github.com/viant/agently/pkg/agently/turn"
)

type (
	Input               = agconv.ConversationInput
	MutableConversation = convw.Conversation
	ConversationHas     = convw.ConversationHas
	MutableMessage      = msgw.Message
	MessageHas          = msgw.MessageHas
	MutableModelCall    = mcall.ModelCall
	ModelCallHas        = mcall.ModelCallHas
	MutableToolCall     = toolcall.ToolCall
	ToolCallHas         = toolcall.ToolCallHas
	MutablePayload      = payloadw.Payload
	PayloadHas          = payloadw.PayloadHas
	MutableTurn         = turnw.Turn
	TurnHas             = turnw.TurnHas
	Payload             = payloadread.PayloadView
)

type (
	Conversation agconv.ConversationView
	Message      agconv.MessageView
	Turn         agconv.TranscriptView
	Transcript   []*Turn
)

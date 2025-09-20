package conversation

import (
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgw "github.com/viant/agently/pkg/agently/message/write"
	mcall "github.com/viant/agently/pkg/agently/modelcall/write"
	payloadread "github.com/viant/agently/pkg/agently/payload/read"
	payloadw "github.com/viant/agently/pkg/agently/payload/write"
	toolcall "github.com/viant/agently/pkg/agently/toolcall/write"
	turnw "github.com/viant/agently/pkg/agently/turn/write"
)

type (
	Input               = agconv.ConversationInput
	MutableConversation = convw.Conversation
	MutableMessage      = msgw.Message
	MutableModelCall    = mcall.ModelCall
	MutableToolCall     = toolcall.ToolCall
	MutablePayload      = payloadw.Payload
	MutableTurn         = turnw.Turn
	Payload             = payloadread.PayloadView
)

type (
	Conversation agconv.ConversationView
	Message      agconv.MessageView
	Turn         agconv.TranscriptView
	Transcript   []*Turn
)

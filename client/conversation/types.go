package conversation

import (
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgw "github.com/viant/agently/pkg/agently/message"
	mcall "github.com/viant/agently/pkg/agently/modelcall"
	payloadw "github.com/viant/agently/pkg/agently/payload"
	payloadread "github.com/viant/agently/pkg/agently/payload/read"
	toolcall "github.com/viant/agently/pkg/agently/toolcall"
)

type (
	Input               = agconv.ConversationInput
	MutableConversation = convw.Conversation
	MutableMessage      = msgw.Message
	MutableModelCall    = mcall.ModelCall
	MutableToolCall     = toolcall.ToolCall
	MutablePayload      = payloadw.Payload
	Payload             = payloadread.PayloadView
)

type (
	Conversation agconv.ConversationView
	Message      agconv.MessageView
	Turn         agconv.TranscriptView
	Transcript   []*Turn
)

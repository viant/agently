package conversation

import (
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgw "github.com/viant/agently/pkg/agently/message/write"
	mcall "github.com/viant/agently/pkg/agently/modelcall/write"
	payloadw "github.com/viant/agently/pkg/agently/payload/write"
	toolcall "github.com/viant/agently/pkg/agently/toolcall/write"
	turnw "github.com/viant/agently/pkg/agently/turn/write"
)

// NewConversation allocates a mutable conversation with Has populated.
func NewConversation() *MutableConversation {
	v := &convw.Conversation{Has: &convw.ConversationHas{}}
	return (*MutableConversation)(v)
}

// NewMessage allocates a mutable message with Has populated.
func NewMessage() *MutableMessage {
	v := &msgw.Message{Has: &msgw.MessageHas{}}
	return (*MutableMessage)(v)
}

// NewModelCall allocates a mutable model call with Has populated.
func NewModelCall() *MutableModelCall {
	v := &mcall.ModelCall{Has: &mcall.ModelCallHas{}}
	return (*MutableModelCall)(v)
}

// NewToolCall allocates a mutable tool call with Has populated.
func NewToolCall() *MutableToolCall {
	v := &toolcall.ToolCall{Has: &toolcall.ToolCallHas{}}
	return (*MutableToolCall)(v)
}

// NewPayload allocates a mutable payload with Has populated.
func NewPayload() *MutablePayload {
	v := &payloadw.Payload{Has: &payloadw.PayloadHas{}}
	return (*MutablePayload)(v)
}

// NewTurn allocates a mutable turn with Has populated.
func NewTurn() *MutableTurn {
	v := &turnw.Turn{Has: &turnw.TurnHas{}}
	return (*MutableTurn)(v)
}

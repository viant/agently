package chat

import (
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	msgw "github.com/viant/agently/pkg/agently/message/write"
	mcall "github.com/viant/agently/pkg/agently/modelcall/write"
	payloadw "github.com/viant/agently/pkg/agently/payload/write"
	toolcall "github.com/viant/agently/pkg/agently/toolcall/write"
	turnw "github.com/viant/agently/pkg/agently/turn/write"
)

func NewConversation() *MutableConversation {
	v := &convw.Conversation{Has: &convw.ConversationHas{}}
	return (*MutableConversation)(v)
}
func NewMessage() *MutableMessage {
	v := &msgw.Message{Has: &msgw.MessageHas{}}
	return (*MutableMessage)(v)
}
func NewModelCall() *MutableModelCall {
	v := &mcall.ModelCall{Has: &mcall.ModelCallHas{}}
	return (*MutableModelCall)(v)
}
func NewToolCall() *MutableToolCall {
	v := &toolcall.ToolCall{Has: &toolcall.ToolCallHas{}}
	return (*MutableToolCall)(v)
}
func NewPayload() *MutablePayload {
	v := &payloadw.Payload{Has: &payloadw.PayloadHas{}}
	return (*MutablePayload)(v)
}
func NewTurn() *MutableTurn { v := &turnw.Turn{Has: &turnw.TurnHas{}}; return (*MutableTurn)(v) }

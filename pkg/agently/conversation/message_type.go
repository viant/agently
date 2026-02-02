package conversation

import "strings"

type MessageType string

const (
	MessageTypeText                MessageType = "text"
	MessageTypeToolOp              MessageType = "tool_op"
	MessageTypeToolResponse        MessageType = "tool_response"
	MessageTypeElicitationRequest  MessageType = "elicitation_request"
	MessageTypeElicitationResponse MessageType = "elicitation_response"
)

func (t MessageType) Normalize() MessageType {
	return MessageType(strings.ToLower(strings.TrimSpace(string(t))))
}

func (t MessageType) IsElicitationRequest() bool {
	return t.Normalize() == MessageTypeElicitationRequest
}

func (t MessageType) IsElicitationResponse() bool {
	return t.Normalize() == MessageTypeElicitationResponse
}

func (t MessageType) IsElicitation() bool {
	n := t.Normalize()
	return n == MessageTypeElicitationRequest || n == MessageTypeElicitationResponse
}

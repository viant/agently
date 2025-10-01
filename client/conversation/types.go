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
	"strings"
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

// UniqueToolNames returns a de-duplicated list of tool names (service/method)
// observed across all messages in the transcript, preserving encounter order.
func (t Transcript) UniqueToolNames() []string {
	if len(t) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, turn := range t {
		if turn == nil || len(turn.Message) == 0 {
			continue
		}
		for _, m := range turn.Message {
			if m == nil || m.ToolCall == nil {
				continue
			}
			name := strings.TrimSpace(m.ToolCall.ToolName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}

package conversation

import (
	"unsafe"

	"github.com/viant/agently/pkg/agently/conversation"
)

type Conversation conversation.ConversationView

func (c *Conversation) GetTranscript() Transcript {
	return *(*Transcript)(unsafe.Pointer(&c.Transcript))
}

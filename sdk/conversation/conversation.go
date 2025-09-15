package conversation

import (
	"unsafe"

	"github.com/viant/agently/pkg/agently/conversation"
)

type Conversation conversation.ConversationView

func (c *Conversation) GetTranscript() Transcript {
	if c.Transcript == nil {
		return nil
	}
	return *(*Transcript)(unsafe.Pointer(&c.Transcript))
}

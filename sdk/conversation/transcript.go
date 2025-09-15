package conversation

import "github.com/viant/agently/pkg/agently/conversation"

type Transcript []*conversation.TranscriptView

func (t *Transcript) Size() int {
	return len(*t)
}

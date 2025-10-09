package conversation

import (
	"strings"

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

func (c *Conversation) HasConversationParent() bool {
	if c.ConversationParentId == nil || *c.ConversationParentId == "" {
		return false
	}
	return true
}

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

func (t Transcript) Last() Transcript {
	if len(t) == 0 {
		return nil
	}
	return t[len(t)-1:]
}

func (m *Message) NewMutable() *MutableMessage {
	// Allocate a fresh mutable message with Has initialized
	out := NewMessage()

	// Required identifiers and always-present fields
	out.SetId(m.Id)
	out.SetConversationID(m.ConversationId)
	out.SetRole(m.Role)
	out.SetType(m.Type)
	out.SetCreatedAt(m.CreatedAt)

	// Optional linkage and ordering
	if m.TurnId != nil {
		out.SetTurnID(*m.TurnId)
	}
	if m.Sequence != nil {
		out.SetSequence(*m.Sequence)
	}
	if m.Archived != nil {
		out.SetArchived(*m.Archived)
	}

	// Timestamps and attribution
	if m.UpdatedAt != nil {
		out.SetUpdatedAt(*m.UpdatedAt)
	}
	if m.CreatedByUserId != nil {
		out.SetCreatedByUserID(*m.CreatedByUserId)
	}

	// Message semantics/content
	if m.Mode != nil {
		out.SetMode(*m.Mode)
	}
	if m.Status != nil {
		out.SetStatus(*m.Status)
	}
	if m.Content != nil {
		out.SetContent(*m.Content)
	}
	out.SetInterim(m.Interim)

	// Optional summaries/tags and relationships
	if m.ContextSummary != nil {
		out.ContextSummary = m.ContextSummary
		if out.Has != nil {
			out.Has.ContextSummary = true
		}
	}
	if m.Tags != nil {
		out.Tags = m.Tags
		if out.Has != nil {
			out.Has.Tags = true
		}
	}
	if m.ElicitationId != nil {
		out.SetElicitationID(*m.ElicitationId)
	}
	if m.ParentMessageId != nil {
		out.SetParentMessageID(*m.ParentMessageId)
	}
	if m.SupersededBy != nil {
		out.SupersededBy = m.SupersededBy
		if out.Has != nil {
			out.Has.SupersededBy = true
		}
	}
	if m.LinkedConversationId != nil {
		out.SetLinkedConversationID(*m.LinkedConversationId)
	}
	if m.AttachmentPayloadId != nil {
		out.SetAttachmentPayloadID(*m.AttachmentPayloadId)
	}
	if m.ElicitationPayloadId != nil {
		out.SetElicitationPayloadID(*m.ElicitationPayloadId)
	}
	if m.ToolName != nil {
		out.SetToolName(*m.ToolName)
	}

	return out
}

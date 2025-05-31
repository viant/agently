package post

type MessageSlice []*Message
type IndexedMessage map[string]*Message

func (c MessageSlice) IndexById() IndexedMessage {
	var result = IndexedMessage{}
	for i, item := range c {
		if item != nil {
			result[item.Id] = c[i]
		}
	}
	return result
}

type ConversationSlice []*Conversation
type IndexedConversation map[string]*Conversation

func (c ConversationSlice) IndexById() IndexedConversation {
	var result = IndexedConversation{}
	for i, item := range c {
		if item != nil {
			result[item.Id] = c[i]
		}
	}
	return result
}

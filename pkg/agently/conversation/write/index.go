package write

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

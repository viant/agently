package post

type ToolCallSlice []*ToolCall
type IndexedToolCall map[int]*ToolCall

func (c ToolCallSlice) IndexById() IndexedToolCall {
	var result = IndexedToolCall{}
	for i, item := range c {
		if item != nil {
			result[item.Id] = c[i]
		}
	}
	return result
}

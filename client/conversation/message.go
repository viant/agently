package conversation

import (
	"sort"
)

func (m *Message) IsInterim() bool {
	if m != nil && m.Interim == 1 {
		return true
	}
	return false
}

type Messages []*Message

// SortByCreatedAt sorts the messages in-place by CreatedAt.
// When asc is true, earlier messages come first; otherwise latest first.
func (m Messages) SortByCreatedAt(asc bool) {
	sort.SliceStable(m, func(i, j int) bool {
		if m[i] == nil || m[j] == nil {
			return false
		}
		if asc {
			return m[i].CreatedAt.Before(m[j].CreatedAt)
		}
		return m[i].CreatedAt.After(m[j].CreatedAt)
	})
}

// SortedByCreatedAt returns a new slice with messages ordered by CreatedAt.
// When asc is true, earlier messages come first; otherwise latest first.
func (m Messages) SortedByCreatedAt(asc bool) Messages {
	out := make(Messages, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	out.SortByCreatedAt(asc)
	return out
}

package cancel

import (
	"context"
	"fmt"
	"sync"
)

// Memory is an in-memory implementation of Registry suitable for single-process runtimes.
type Memory struct {
	mu            sync.Mutex
	cancelsByTurn map[string][]context.CancelFunc // key: user turn id (message id)
	turnsByConv   map[string][]string             // convID -> []turnID
}

// NewMemory returns a new in-memory cancel registry.
func NewMemory() *Memory { return &Memory{} }

func (m *Memory) Register(convID, turnID string, cancel context.CancelFunc) {
	if m == nil || cancel == nil || turnID == "" {
		return
	}
	m.mu.Lock()
	if m.cancelsByTurn == nil {
		m.cancelsByTurn = map[string][]context.CancelFunc{}
	}
	m.cancelsByTurn[turnID] = append(m.cancelsByTurn[turnID], cancel)
	if convID != "" {
		if m.turnsByConv == nil {
			m.turnsByConv = map[string][]string{}
		}
		m.turnsByConv[convID] = append(m.turnsByConv[convID], turnID)
	}
	m.mu.Unlock()
}

func (m *Memory) Complete(convID, turnID string, cancel context.CancelFunc) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.cancelsByTurn != nil {
		list := m.cancelsByTurn[turnID]
		for i, c := range list {
			if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", cancel) {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(m.cancelsByTurn, turnID)
		} else {
			m.cancelsByTurn[turnID] = list
		}
	}
	m.mu.Unlock()
}

func (m *Memory) CancelTurn(turnID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	var list []context.CancelFunc
	if m.cancelsByTurn != nil {
		list = m.cancelsByTurn[turnID]
		delete(m.cancelsByTurn, turnID)
	}
	m.mu.Unlock()
	for _, c := range list {
		if c != nil {
			c()
		}
	}
	return len(list) > 0
}

func (m *Memory) CancelConversation(convID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	var result []context.CancelFunc
	if m.turnsByConv != nil && m.cancelsByTurn != nil {
		turns := m.turnsByConv[convID]
		delete(m.turnsByConv, convID)
		for _, tID := range turns {
			if list, ok := m.cancelsByTurn[tID]; ok {
				result = append(result, list...)
				delete(m.cancelsByTurn, tID)
			}
		}
	}
	m.mu.Unlock()
	for _, c := range result {
		if c != nil {
			c()
		}
	}
	return len(result) > 0
}

package log

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// EventType represents classification of an event.
type EventType string

const (
	LLMInput   EventType = "LLM_INPUT"
	LLMOutput  EventType = "LLM_OUTPUT"
	TaskInput  EventType = "TASK_INPUT"
	TaskOutput EventType = "TASK_OUTPUT"
	ToolInput  EventType = "TOOL_INPUT"
	ToolOutput EventType = "TOOL_OUTPUT"
	TaskWhen   EventType = "TASK_WHEN"
)

type Event struct {
	Time      time.Time   `json:"ts"`
	EventType EventType   `json:"eventtype"`
	Payload   interface{} `json:"p"`
}

// Collector collects events and fans them out to subscribers.
type Collector struct {
	mu   sync.RWMutex
	subs []chan Event
}

var Default = &Collector{}

// Publish sends an event to all subscribers (non-blocking).
func Publish(e Event) {
	Default.Publish(e)
}

func (c *Collector) Publish(e Event) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, ch := range c.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a receive-only channel for events. buf is channel size.
func (c *Collector) Subscribe(buf int) <-chan Event {
	ch := make(chan Event, buf)
	c.mu.Lock()
	c.subs = append(c.subs, ch)
	c.mu.Unlock()
	return ch
}

// FileSink writes every event (JSON encoded) to w, filtering by event types if provided.
func FileSink(w io.Writer, filters ...EventType) {
	want := map[EventType]bool{}
	for _, f := range filters {
		want[f] = true
	}
	go func() {
		enc := json.NewEncoder(w)
		for ev := range Default.Subscribe(100) {
			if len(want) > 0 && !want[ev.EventType] {
				continue
			}
			_ = enc.Encode(ev)
		}
	}()
}

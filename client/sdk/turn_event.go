package sdk

import (
	"context"
	"strings"
	"time"
)

const (
	TurnEventDelta       = "delta"
	TurnEventAssistant   = "assistant_message"
	TurnEventInterim     = "interim_message"
	TurnEventElicitation = "elicitation"
	TurnEventTool        = "tool"
)

// TurnEvent is a normalized SDK-level event emitted by StreamTurnEvents.
// It intentionally hides provider-specific payloads and focuses on
// conversation-visible semantics.
type TurnEvent struct {
	Type           string
	ConversationID string
	TurnID         string
	MessageID      string
	Role           string
	MessageType    string
	CreatedAt      time.Time

	// Text holds the incremental delta for assistant output.
	// TextFull holds the reconstructed full text (best effort).
	Text     string
	TextFull string

	// Tool events (if present).
	ToolName  string
	ToolPhase string

	// Elicitation payload (if present).
	Elicitation *Elicitation
}

// StreamTurnEvents wraps StreamEventsWithOptions and emits normalized events.
// It only returns assistant deltas, tool events, and elicitation events.
func (c *Client) StreamTurnEvents(ctx context.Context, conversationID string, since string, include []string, includeHistory bool) (<-chan *TurnEvent, <-chan error, error) {
	events, errs, err := c.StreamEventsWithOptions(ctx, conversationID, since, include, includeHistory)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan *TurnEvent, 64)
	outErrs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(outErrs)
		buf := NewMessageBuffer()
		lastByMsg := map[string]string{}
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				if ev == nil || ev.Message == nil {
					continue
				}
				if ev.Event.IsElicitation() || IsElicitationPending(ev.Message) {
					if el := ElicitationFromEvent(ev); el != nil {
						out <- &TurnEvent{
							Type:           TurnEventElicitation,
							ConversationID: ev.ConversationID,
							TurnID:         strings.TrimSpace(ptrString(ev.Message.TurnId)),
							MessageID:      strings.TrimSpace(ev.Message.Id),
							Role:           strings.TrimSpace(ev.Message.Role),
							MessageType:    strings.TrimSpace(ev.Message.Type),
							CreatedAt:      ev.Message.CreatedAt,
							Elicitation:    el,
						}
					}
					continue
				}
				phase := ToolPhase(ev)
				if phase == "" {
					phase = ToolPhaseFromEvent(ev)
				}
				if name := ToolName(ev); name != "" || ev.Event.normalize() == StreamEventToolCallStarted || ev.Event.normalize() == StreamEventToolCallCompleted || ev.Event.normalize() == StreamEventToolCallFailed {
					if name == "" && ev.Message != nil && ev.Message.ToolCall != nil {
						name = strings.TrimSpace(ev.Message.ToolCall.ToolName)
					}
					out <- &TurnEvent{
						Type:           TurnEventTool,
						ConversationID: ev.ConversationID,
						TurnID:         strings.TrimSpace(ptrString(ev.Message.TurnId)),
						MessageID:      strings.TrimSpace(ev.Message.Id),
						Role:           strings.TrimSpace(ev.Message.Role),
						MessageType:    strings.TrimSpace(ev.Message.Type),
						CreatedAt:      ev.Message.CreatedAt,
						ToolName:       name,
						ToolPhase:      phase,
					}
				}
				if strings.ToLower(strings.TrimSpace(ev.Message.Role)) != "assistant" {
					continue
				}
				msgID, text, changed := buf.ApplyEvent(ev)
				if !changed || strings.TrimSpace(text) == "" {
					continue
				}
				if strings.TrimSpace(text) == "" {
					continue
				}
				prev := lastByMsg[msgID]
				delta := deltaFrom(prev, text)
				if delta == "" {
					continue
				}
				lastByMsg[msgID] = text
				eventType := TurnEventDelta
				if ev.Event.IsAssistantMessage() {
					eventType = TurnEventAssistant
				} else if ev.Event.IsInterimMessage() {
					eventType = TurnEventInterim
				}
				out <- &TurnEvent{
					Type:           eventType,
					ConversationID: ev.ConversationID,
					TurnID:         strings.TrimSpace(ptrString(ev.Message.TurnId)),
					MessageID:      strings.TrimSpace(ev.Message.Id),
					Role:           strings.TrimSpace(ev.Message.Role),
					MessageType:    strings.TrimSpace(ev.Message.Type),
					CreatedAt:      ev.Message.CreatedAt,
					Text:           delta,
					TextFull:       text,
				}
			case err, ok := <-errs:
				if !ok {
					return
				}
				if err != nil {
					outErrs <- err
					return
				}
			case <-ctx.Done():
				outErrs <- ctx.Err()
				return
			}
		}
	}()
	return out, outErrs, nil
}

func deltaFrom(prev, curr string) string {
	if curr == prev {
		return ""
	}
	if strings.HasPrefix(curr, prev) {
		return curr[len(prev):]
	}
	return curr
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

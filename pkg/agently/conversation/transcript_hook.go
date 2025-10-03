package conversation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/viant/agently/genai/tool"
)

func (t *TranscriptView) filterInvokedToolFeed() {
	var cloned = make([]*tool.Feed, 0, len(t.ToolFeed))
	for _, feed := range t.ToolFeed {
		if feed.Invoked {
			continue
		}
		cloned = append(cloned, feed)
	}
	t.ToolFeed = cloned
}

// OnRelation is invoked after related records are loaded.
// Ensure messages are ordered by CreatedAt ascending (oldest first) and compute Turn stage.
func (t *TranscriptView) OnRelation(ctx context.Context) {

	// Normalize messages when present to ensure deterministic order and elapsed time
	if len(t.Message) > 0 {
		t.normalizeMessages()
	}
	// Always attempt to compute tool executions. For activation.kind==tool_call
	// we may still want to invoke even when there are no recorded tool calls.
	var err error
	t.ToolFeed, err = t.computeToolFeed(ctx)
	if err != nil {
		fmt.Printf("failed to compute tool feed: %v\n", err)
	}
	// Compute stage for this turn
	t.Stage = computeTurnStage(t)
}

func (t *TranscriptView) normalizeMessages() {
	// Stable sort to preserve original order for equal timestamps.
	sort.SliceStable(t.Message, func(i, j int) bool {
		mi, mj := t.Message[i], t.Message[j]
		if mi == nil || mj == nil {
			// Keep non-nil before nil
			return mj == nil && mi != nil
		}
		if mi.CreatedAt.Equal(mj.CreatedAt) {
			if mi.ToolCall != nil {
				return true
			}
			// Use Sequence if available as a tie breaker
			if mi.Sequence != nil && mj.Sequence != nil {
				return *mi.Sequence < *mj.Sequence
			}
			// Fall back to ID to ensure deterministic ordering
			return mi.Id < mj.Id
		}
		return mi.CreatedAt.Before(mj.CreatedAt)
	})
	minTime := t.Message[0].CreatedAt
	maxTime := t.Message[len(t.Message)-1].CreatedAt
	t.ElapsedInSec = int(maxTime.Sub(minTime).Seconds())
	for _, m := range t.Message {
		if m.ModelCall != nil {
			m.Status = &m.ModelCall.Status
		}
		if m.ToolCall != nil {
			m.Status = &m.ToolCall.Status
		}
	}
}

// computeTurnStage determines the stage of a single turn based on its latest non-interim message.
func computeTurnStage(t *TranscriptView) string {
	if t == nil || len(t.Message) == 0 {
		return StageWaiting
	}
	// If turn itself is canceled, treat as completed
	if strings.EqualFold(strings.TrimSpace(t.Status), "canceled") {
		return StageDone
	}
	lastRole := ""
	lastAssistantElic := false
	lastToolRunning := false
	lastToolFailed := false
	lastModelRunning := false
	lastAssistantCanceled := false

	// Iterate messages backwards to find cancellation or the latest non-interim one
	for i := len(t.Message) - 1; i >= 0; i-- {
		m := t.Message[i]
		if m == nil {
			continue
		}
		// If the latest assistant message is explicitly canceled (even interim), drop to waiting
		if strings.EqualFold(strings.TrimSpace(m.Role), "assistant") && m.Status != nil && strings.EqualFold(strings.TrimSpace(*m.Status), "canceled") {
			lastAssistantCanceled = true
			break
		}
		if m.Interim != 0 {
			continue
		}
		r := strings.ToLower(strings.TrimSpace(m.Role))
		if lastRole == "" {
			lastRole = r
		}
		if m.ToolCall != nil {
			status := strings.ToLower(strings.TrimSpace(m.ToolCall.Status))
			if status == "running" || m.ToolCall.CompletedAt == nil {
				lastToolRunning = true
			}
			if status == "failed" {
				lastToolFailed = true
			}
		}
		if m.ModelCall != nil {
			mstatus := strings.ToLower(strings.TrimSpace(m.ModelCall.Status))
			if mstatus == "running" || m.ModelCall.CompletedAt == nil {
				lastModelRunning = true
			}
		}
		if r == "assistant" && m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
			lastAssistantElic = true
		}
		break
	}

	switch {
	case lastAssistantCanceled:
		return StageDone
	case lastToolRunning:
		return StageExecuting
	case lastAssistantElic:
		return StageEliciting
	case lastModelRunning:
		return StageThinking
	case lastRole == "user":
		return StageThinking
	case lastToolFailed:
		return StageError
	default:
		return StageDone
	}
}

package conversation

import (
	"context"
	"sort"
	"strings"
)

// OnRelation is invoked after related records are loaded.
// Ensure messages are ordered by CreatedAt ascending (oldest first) and compute Turn stage.
func (t *TranscriptView) OnRelation(ctx context.Context) {
	if t == nil {
		return
	}
	if len(t.Message) > 1 {
		// Stable sort to preserve original order for equal timestamps.
		sort.SliceStable(t.Message, func(i, j int) bool {
			mi, mj := t.Message[i], t.Message[j]
			if mi == nil || mj == nil {
				// Keep non-nil before nil
				return mj == nil && mi != nil
			}
			if mi.CreatedAt.Equal(mj.CreatedAt) {
				// Use Sequence if available as a tie breaker
				if mi.Sequence != nil && mj.Sequence != nil {
					return *mi.Sequence < *mj.Sequence
				}
				// Fall back to ID to ensure deterministic ordering
				return mi.Id < mj.Id
			}
			return mi.CreatedAt.Before(mj.CreatedAt)
		})
	}
	// Compute stage for this turn
	t.Stage = computeTurnStage(t)
}

// computeTurnStage determines the stage of a single turn based on its latest non-interim message.
func computeTurnStage(t *TranscriptView) string {
	if t == nil || len(t.Message) == 0 {
		return StageWaiting
	}
	lastRole := ""
	lastAssistantElic := false
	lastToolRunning := false
	lastToolFailed := false
	lastModelRunning := false

	// Iterate messages backwards to find the latest non-interim one
	for i := len(t.Message) - 1; i >= 0; i-- {
		m := t.Message[i]
		if m == nil {
			continue
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

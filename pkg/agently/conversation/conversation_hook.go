package conversation

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

func (c *ConversationView) OnRelation(ctx context.Context) {
	// Stable sort to preserve original order for equal timestamps.
	sort.SliceStable(c.Transcript, func(i, j int) bool {
		mi, mj := c.Transcript[i], c.Transcript[j]
		if mi == nil || mj == nil {
			// Keep non-nil before nil
			return mj == nil && mi != nil
		}
		if mi.CreatedAt.Equal(mj.CreatedAt) {
			// Fall back to ID to ensure deterministic ordering
			return mi.Id < mj.Id
		}
		return mi.CreatedAt.Before(mj.CreatedAt)
	})

	for i := 0; i < len(c.Transcript)-2; i++ {
		c.Transcript[i].filterInvokedToolFeed()
	}
	// Compute live stage based on latest transcript signals
	c.Stage = computeStage(c)
}

func computeStage(c *ConversationView) string {

	if c == nil || len(c.Transcript) == 0 {
		return StageWaiting
	}
	lastRole := ""
	lastAssistantElic := false
	lastToolRunning := false
	lastToolFailed := false
	lastModelRunning := false
	lastAssistantCanceled := false

	// Iterate turns backwards, then messages backwards within the turn
	for ti := len(c.Transcript) - 1; ti >= 0; ti-- {
		t := c.Transcript[ti]
		if t == nil || len(t.Message) == 0 {
			continue
		}
		// If entire turn was canceled, treat conversation as completed
		if strings.EqualFold(strings.TrimSpace(t.Status), "canceled") {
			return StageDone
		}
		for mi := len(t.Message) - 1; mi >= 0; mi-- {
			m := t.Message[mi]
			if m == nil {
				continue
			}
			// If latest assistant message is canceled (even interim), drop to waiting
			if strings.EqualFold(strings.TrimSpace(m.Role), "assistant") && m.Status != nil && strings.EqualFold(strings.TrimSpace(*m.Status), "canceled") {
				lastAssistantCanceled = true
				goto DONE
			}
			// Skip interim entries for other evaluations
			if m.Interim != 0 {
				continue
			}
			r := strings.ToLower(strings.TrimSpace(m.Role))
			if lastRole == "" {
				lastRole = r
			}
			// Evaluate the latest non-interim message only
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
			// Break after inspecting the latest eligible message
			goto DONE
		}
	}
DONE:
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
		// We had some messages but no running signals
		return StageDone
	}
}

var ConversationsPathURI = "/v1/api/agently/conversation/"

func DefineConversationsComponent(ctx context.Context, srv *datly.Service) error {
	aComponent, err := repository.NewComponent(
		contract.NewPath("GET", ConversationsPathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(ConversationInput{}),
			reflect.TypeOf(ConversationOutput{}), &ConversationFS, view.WithConnectorRef("agently")))

	if err != nil {
		return fmt.Errorf("failed to create Conversation component: %w", err)
	}
	if err := srv.AddComponent(ctx, aComponent); err != nil {
		return fmt.Errorf("failed to add Conversation component: %w", err)
	}
	return nil
}

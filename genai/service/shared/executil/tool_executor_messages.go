package executil

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// createToolMessage persists a new tool message and returns its ID.
func createToolMessage(ctx context.Context, conv apiconv.Client, turn memory.TurnMeta, startedAt time.Time) (string, error) {
	toolMsgID := uuid.New().String()
	opts := []apiconv.MessageOption{
		apiconv.WithId(toolMsgID),
		apiconv.WithRole("tool"),
		apiconv.WithType("tool_op"),
		apiconv.WithCreatedAt(startedAt),
	}
	if IsChainMode(ctx) {
		opts = append(opts, apiconv.WithMode("chain"))
	}
	msg, err := apiconv.AddMessage(ctx, conv, &turn, opts...)
	if err != nil {
		return "", fmt.Errorf("persist tool message: %w", err)
	}
	return msg.Id, nil
}

// initToolCall initializes and persists a new tool call in a 'running' state for the given tool message.
func initToolCall(ctx context.Context, conv apiconv.Client, toolMsgID, opID string, turn memory.TurnMeta, toolName string, startedAt time.Time, traceID string) error {
	tc := apiconv.NewToolCall()
	tc.SetMessageID(toolMsgID)
	if opID != "" {
		tc.SetOpID(opID)
	}
	if turn.TurnID != "" {
		tc.TurnID = &turn.TurnID
		tc.Has.TurnID = true
	}
	tc.SetToolName(toolName)
	tc.SetToolKind("general")
	tc.SetStatus("running")

	now := startedAt
	tc.StartedAt = &now
	tc.Has.StartedAt = true
	if trace := strings.TrimSpace(traceID); trace != "" {
		tc.TraceID = &trace
		tc.Has.TraceID = true
	} else if trace := strings.TrimSpace(memory.TurnTrace(turn.TurnID)); trace != "" {
		tc.TraceID = &trace
		tc.Has.TraceID = true
	}
	if err := conv.PatchToolCall(ctx, tc); err != nil {
		return fmt.Errorf("persist tool call start: %w", err)
	}

	if err := conv.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, "running")); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	return nil
}

// completeToolCall marks the tool call as finished and attaches the response payload and error message.
func completeToolCall(ctx context.Context, conv apiconv.Client, toolMsgID, status string, completedAt time.Time, respPayloadID string, errMsg string) error {
	updTC := apiconv.NewToolCall()
	updTC.SetMessageID(toolMsgID)
	updTC.SetStatus(status)
	done := completedAt
	updTC.CompletedAt = &done
	updTC.Has.CompletedAt = true
	if respPayloadID != "" {
		updTC.ResponsePayloadID = &respPayloadID
		updTC.Has.ResponsePayloadID = true
	}
	if errMsg != "" {
		updTC.ErrorMessage = &errMsg
		updTC.Has.ErrorMessage = true
	}

	return conv.PatchToolCall(ctx, updTC)
}

package executil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	plan "github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// StepInfo carries the tool step data needed for execution.
type StepInfo struct {
	ID   string
	Name string
	Args map[string]interface{}
}

// ExecuteToolStep runs a tool via the registry, records transcript, and updates traces.
// Returns normalized plan.ToolCall, span and any combined error.
func ExecuteToolStep(ctx context.Context, reg tool.Registry, step StepInfo, conv apiconv.Client) (plan.ToolCall, plan.CallSpan, error) {
	span := plan.CallSpan{StartedAt: time.Now()}
	errs := make([]error, 0, 6)

	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return plan.ToolCall{}, span, fmt.Errorf("turn meta not found")
	}

	// 1) Create tool message
	toolMsgID, err := createToolMessage(ctx, conv, turn, span.StartedAt)
	if err != nil {
		return plan.ToolCall{}, span, err
	}

	// 2) Initialize tool call (running) with LLM op id
	if err := initToolCall(ctx, conv, toolMsgID, step.ID, turn, step.Name, span.StartedAt); err != nil {
		return plan.ToolCall{}, span, err
	}

	// 3) Persist request payload
	if len(step.Args) > 0 {
		if _, pErr := persistRequestPayload(ctx, conv, toolMsgID, step.Args); pErr != nil {
			errs = append(errs, fmt.Errorf("persist request payload: %w", pErr))
		}
	}

	// 4) Execute tool
	out, toolResult, execErr := executeTool(ctx, reg, step)
	if execErr != nil {
		errs = append(errs, fmt.Errorf("execute tool: %w", execErr))
	}
	span.SetEnd(time.Now())

	// 5) Persist response payload (use background when canceled to avoid DB write cancellation)
	persistCtx := ctx
	if ctx.Err() == context.Canceled {
		persistCtx = context.Background()
	}
	respID, respErr := persistResponsePayload(persistCtx, conv, toolResult)
	if respErr != nil {
		errs = append(errs, fmt.Errorf("persist response payload: %w", respErr))
	}

	// 6) Update tool message with result content - why duplication of content gere
	//if uErr := updateToolMessageContent(persistCtx, conv, toolMsgID, toolResult); uErr != nil {
	//	errs = append(errs, fmt.Errorf("update tool message: %w", uErr))
	//}

	// 7) Finish tool call (record error message when present)
	status := "completed"
	var errMsg string
	// Detect cancellation originating from context
	if execErr != nil {
		if errors.Is(execErr, context.Canceled) || ctx.Err() == context.Canceled {
			status = "canceled"
		} else {
			status = "failed"
			errMsg = execErr.Error()
		}
	} else if ctx.Err() == context.Canceled {
		status = "canceled"
	}
	// Use background for final write when terminated to avoid canceled ctx
	finCtx := ctx
	if status == "canceled" {
		finCtx = context.Background()
	}
	if cErr := completeToolCall(finCtx, conv, toolMsgID, status, span.EndedAt, respID, errMsg); cErr != nil {
		errs = append(errs, fmt.Errorf("complete tool call: %w", cErr))
	}
	_ = conv.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, status))
	var retErr error
	if len(errs) > 0 {
		retErr = errors.Join(errs...)
	}
	return out, span, retErr
}

// createToolMessage persists a new tool message and returns its ID.
func createToolMessage(ctx context.Context, conv apiconv.Client, turn memory.TurnMeta, startedAt time.Time) (string, error) {
	toolMsgID := uuid.New().String()
	msg, err := apiconv.AddMessage(ctx, conv, &turn,
		apiconv.WithId(toolMsgID),
		apiconv.WithRole("tool"),
		apiconv.WithType("tool_op"),
		apiconv.WithCreatedAt(startedAt),
	)
	if err != nil {
		return "", fmt.Errorf("persist tool message: %w", err)
	}
	return msg.Id, nil
}

// initToolCall initializes and persists a new tool call in a 'running' state for the given tool message.
func initToolCall(ctx context.Context, conv apiconv.Client, toolMsgID, opID string, turn memory.TurnMeta, toolName string, startedAt time.Time) error {
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
	if err := conv.PatchToolCall(ctx, tc); err != nil {
		return fmt.Errorf("persist tool call start: %w", err)
	}

	if err := conv.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, "running")); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	return nil
}

// executeTool runs the tool and returns the normalized ToolCall, raw result and error.
func executeTool(ctx context.Context, reg tool.Registry, step StepInfo) (plan.ToolCall, string, error) {
	toolResult, err := reg.Execute(ctx, step.Name, step.Args)
	out := plan.ToolCall{ID: step.ID, Name: step.Name, Arguments: step.Args, Result: toolResult}
	if err != nil {
		out.Error = err.Error()
	}
	return out, toolResult, err
}

// persistRequestPayload stores tool request arguments and links them to the tool call.
func persistRequestPayload(ctx context.Context, conv apiconv.Client, toolMsgID string, args map[string]interface{}) (string, error) {
	b, mErr := json.Marshal(args)
	if mErr != nil {
		return "", mErr
	}
	// create payload using shared helper
	reqID, pErr := createInlinePayload(ctx, conv, "tool_request", "application/json", b)
	if pErr != nil {
		return "", pErr
	}
	// link payload to tool call
	upd := apiconv.NewToolCall()
	upd.SetMessageID(toolMsgID)
	upd.RequestPayloadID = &reqID
	upd.Has.RequestPayloadID = true
	_ = conv.PatchToolCall(ctx, upd)
	return reqID, nil
}

// persistResponsePayload stores tool response content and returns the payload ID.
func persistResponsePayload(ctx context.Context, conv apiconv.Client, result string) (string, error) {
	rb := []byte(result)
	return createInlinePayload(ctx, conv, "tool_response", "text/plain", rb)
}

// updateToolMessageContent updates the tool message content with the given result.
func updateToolMessageContent(ctx context.Context, conv apiconv.Client, toolMsgID string, content string) error {
	updMsg := apiconv.NewMessage()
	updMsg.SetId(toolMsgID)
	updMsg.SetContent(content)
	return conv.PatchMessage(ctx, updMsg)
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
	if strings := errMsg; strings != "" {
		updTC.ErrorMessage = &strings
		updTC.Has.ErrorMessage = true
	}
	return conv.PatchToolCall(ctx, updTC)
}

// createInlinePayload creates and persists an inline payload and returns its ID.
func createInlinePayload(ctx context.Context, conv apiconv.Client, kind, mime string, body []byte) (string, error) {
	pid := uuid.New().String()
	p := apiconv.NewPayload()
	p.SetId(pid)
	p.SetKind(kind)
	p.SetMimeType(mime)
	p.SetSizeBytes(len(body))
	p.SetStorage("inline")
	p.SetInlineBody(body)
	if err := conv.PatchPayload(ctx, p); err != nil {
		return "", err
	}
	return pid, nil
}

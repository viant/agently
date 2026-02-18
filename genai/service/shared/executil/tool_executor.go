package executil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	apiconv "github.com/viant/agently/client/conversation"
	plan "github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

const (
	SystemDocumentTag  = "system_doc"
	SystemDocumentMode = "system_document"
)

// StepInfo carries the tool step data needed for execution.
type StepInfo struct {
	ID   string
	Name string
	Args map[string]interface{}
	// ResponseID is the assistant response.id that requested this tool call
	ResponseID string
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
	argsJSON := ""
	if debugConvEnabled() && len(step.Args) > 0 {
		if b, jErr := json.Marshal(step.Args); jErr == nil {
			argsJSON = string(b)
		} else {
			argsJSON = fmt.Sprintf("{\"marshal_error\":%q}", jErr.Error())
		}
	}
	debugConvf("tool execute start convo=%q turn=%q op_id=%q tool=%q args_len=%d args_head=%q args_tail=%q", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), len(argsJSON), headString(argsJSON, 512), tailString(argsJSON, 512))

	// 1) Create tool message
	toolMsgID, err := createToolMessage(ctx, conv, turn, span.StartedAt)
	if err != nil {
		return plan.ToolCall{}, span, err
	}

	// 2) Initialize tool call (running) with LLM op id
	if err := initToolCall(ctx, conv, toolMsgID, step.ID, turn, step.Name, span.StartedAt, step.ResponseID); err != nil {
		return plan.ToolCall{}, span, err
	}

	// 3) Persist request payload
	if len(step.Args) > 0 {
		if _, pErr := persistRequestPayload(ctx, conv, toolMsgID, step.Args); pErr != nil {
			errs = append(errs, fmt.Errorf("persist request payload: %w", pErr))
		}
	}

	// 4) Execute tool with a bounded context so one stuck call won't hang the run
	// Apply per-tool timeout when available (scoped registry exposes TimeoutResolver directly).
	if tr, ok := reg.(tool.TimeoutResolver); ok {
		if d, ok2 := tr.ToolTimeout(step.Name); ok2 && d > 0 {
			ctx = WithToolTimeout(ctx, d)
		}
	}
	out, toolResult, execErr := executeToolWithRetry(ctx, reg, step)
	// Optionally wrap overflow with YAML helper when native continuation is not supported.
	if wrapped := maybeWrapOverflow(ctx, reg, step.Name, toolResult, toolMsgID); wrapped != "" {
		toolResult = wrapped
		out.Result = wrapped
	}
	if execErr != nil && strings.TrimSpace(toolResult) == "" {
		// Provide the error text as response payload so the LLM can reason over it.
		toolResult = execErr.Error()
		out.Result = toolResult
	}
	if execErr != nil {
		errs = append(errs, fmt.Errorf("execute tool: %w", execErr))
	}
	span.SetEnd(time.Now())

	// 5) Persist side effects + response payload (use background when canceled to avoid DB write cancellation)
	persistCtx := ctx
	if ctx.Err() == context.Canceled {
		persistCtx = context.Background()
	}

	if strings.TrimSpace(toolResult) != "" {
		docCtx := ctx
		if ctx.Err() != nil {
			docCtx = context.Background()
		}
		if err := persistDocumentsIfNeeded(docCtx, reg, conv, turn, step.Name, toolResult); err != nil {
			errs = append(errs, fmt.Errorf("emit system content: %w", err))
		}
		if err := persistToolImageAttachmentIfNeeded(docCtx, conv, turn, toolMsgID, step.Name, toolResult); err != nil {
			errs = append(errs, fmt.Errorf("persist tool attachments: %w", err))
		}
		if redacted, ok := redactToolResultIfNeeded(step.Name, toolResult); ok {
			toolResult = redacted
			out.Result = redacted
		}
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
	status, errMsg := resolveToolStatus(execErr, ctx)
	// Use background for final write when terminated to avoid canceled ctx
	finCtx := ctx
	if status == "canceled" {
		finCtx = context.Background()
	}
	if cErr := completeToolCall(finCtx, conv, toolMsgID, status, span.EndedAt, respID, errMsg); cErr != nil {
		errs = append(errs, fmt.Errorf("complete tool call: %w", cErr))
	}
	patchErr := conv.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, status))
	errs = append(errs, fmt.Errorf("patch conversations call: %w", patchErr))

	var retErr error
	if len(errs) > 0 {
		retErr = errors.Join(errs...)
	}

	if retErr != nil {
		errorConvf("tool execute done convo=%q turn=%q op_id=%q tool=%q status=%q result_len=%d err=%v", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(status), len(toolResult), retErr)
	} else {
		infoConvf("tool execute done convo=%q turn=%q op_id=%q tool=%q status=%q result_len=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(status), len(toolResult))
	}

	return out, span, retErr
}

// SynthesizeToolStep persists a tool call using a precomputed result without
// invoking the actual tool. It mirrors ExecuteToolStep's persistence flow
// (messages, request/response payloads, status), setting status to "completed".
func SynthesizeToolStep(ctx context.Context, conv apiconv.Client, step StepInfo, toolResult string) error {
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return fmt.Errorf("turn meta not found")
	}
	argsJSON := ""
	if debugConvEnabled() && len(step.Args) > 0 {
		if b, jErr := json.Marshal(step.Args); jErr == nil {
			argsJSON = string(b)
		} else {
			argsJSON = fmt.Sprintf("{\"marshal_error\":%q}", jErr.Error())
		}
	}
	debugConvf("tool synth start convo=%q turn=%q op_id=%q tool=%q args_len=%d args_head=%q args_tail=%q result_len=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), len(argsJSON), headString(argsJSON, 512), tailString(argsJSON, 512), len(toolResult))
	startedAt := time.Now()
	toolMsgID, err := createToolMessage(ctx, conv, turn, startedAt)
	if err != nil {
		return err
	}
	if err := initToolCall(ctx, conv, toolMsgID, step.ID, turn, step.Name, startedAt, step.ResponseID); err != nil {
		return err
	}
	if len(step.Args) > 0 {
		if _, pErr := persistRequestPayload(ctx, conv, toolMsgID, step.Args); pErr != nil {
			return fmt.Errorf("persist request payload: %w", pErr)
		}
	}
	// Persist provided result
	if redacted, ok := redactToolResultIfNeeded(step.Name, toolResult); ok {
		toolResult = redacted
	}
	respID, respErr := persistResponsePayload(ctx, conv, toolResult)
	if respErr != nil {
		return fmt.Errorf("persist response payload: %w", respErr)
	}
	// Complete tool call
	status := "completed"
	completedAt := time.Now()
	if cErr := completeToolCall(ctx, conv, toolMsgID, status, completedAt, respID, ""); cErr != nil {
		return fmt.Errorf("complete tool call: %w", cErr)
	}
	_ = conv.PatchConversations(ctx, convw.NewConversationStatus(turn.ConversationID, status))
	debugConvf("tool synth done convo=%q turn=%q op_id=%q tool=%q status=%q result_len=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(status), len(toolResult))
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

// resolveToolStatus determines the terminal status for a tool call based on execution error and parent context.
// Returns one of: "completed", "failed", "canceled" and an optional error message.
func resolveToolStatus(execErr error, parentCtx context.Context) (string, string) {
	status := "completed"
	var errMsg string
	if execErr != nil {
		// Treat context cancellation and deadline as cancellations, not failures
		if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) || parentCtx.Err() == context.Canceled {
			status = "canceled"
		} else {
			status = "failed"
			errMsg = execErr.Error()
		}
	} else if parentCtx.Err() == context.Canceled {
		status = "canceled"
	}
	return status, errMsg
}

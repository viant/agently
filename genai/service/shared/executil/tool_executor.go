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
	SystemDocumentTag    = "system_doc"
	SystemDocumentMode   = "system_document"
	ResourceDocumentTag  = "resource_doc"
	finalizeWriteTimeout = 15 * time.Second
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
func ExecuteToolStep(ctx context.Context, reg tool.Registry, step StepInfo, conv apiconv.Client) (out plan.ToolCall, span plan.CallSpan, retErr error) {
	span = plan.CallSpan{StartedAt: time.Now()}
	errs := make([]error, 0, 6)

	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		retErr = fmt.Errorf("turn meta not found")
		return
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
	toolMsgID := ""
	toolCallStarted := false
	toolCallClosed := false
	conversationPatched := false
	forcedStatus := ""
	// Ensure started tool calls never remain non-terminal on abort/early exits.
	// We also patch conversation status independently because it is the aggregate
	// status shown in UI/history and can drift if only tool_call is closed.
	// Running both writes (tool_call + conversation) keeps local call truth and
	// top-level conversation truth aligned as much as possible.
	defer func() {
		if !toolCallStarted || strings.TrimSpace(toolMsgID) == "" {
			return
		}
		status := strings.TrimSpace(forcedStatus)
		if status == "" {
			if errors.Is(retErr, context.Canceled) || errors.Is(retErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				status = "canceled"
			} else {
				status = "failed"
			}
		}
		errMsg := ""
		if status == "failed" {
			if retErr != nil {
				errMsg = retErr.Error()
			} else if cerr := ctx.Err(); cerr != nil {
				errMsg = cerr.Error()
			} else {
				errMsg = "forced close on abort"
			}
		}
		finCtx, cancelFin := detachedFinalizeCtx(ctx)
		defer cancelFin()
		warnConvf("tool force close convo=%q turn=%q op_id=%q tool=%q status=%q ret_err=%q parent_ctx_err=%q", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(status), strings.TrimSpace(errMsg), strings.TrimSpace(formatContextErr(ctx)))
		if !toolCallClosed {
			_ = completeToolCall(finCtx, conv, toolMsgID, status, time.Now(), "", errMsg)
		}
		if !conversationPatched {
			_ = conv.PatchConversations(finCtx, convw.NewConversationStatus(turn.ConversationID, status))
		}
	}()

	// 1) Create tool message
	toolMsgID, err := createToolMessage(ctx, conv, turn, span.StartedAt)
	if err != nil {
		retErr = err
		return
	}

	// 2) Initialize tool call (running) with LLM op id
	if err := initToolCall(ctx, conv, toolMsgID, step.ID, turn, step.Name, span.StartedAt, step.ResponseID); err != nil {
		retErr = err
		return
	}
	toolCallStarted = true

	// 3) Persist request payload
	if len(step.Args) > 0 {
		if _, pErr := persistRequestPayload(ctx, conv, toolMsgID, step.Args); pErr != nil {
			errs = append(errs, fmt.Errorf("persist request payload: %w", pErr))
		}
	}

	// 4) Execute tool with a bounded context so one stuck call won't hang the run
	// Apply per-tool timeout when available (scoped registry exposes TimeoutResolver directly).
	registryTimeout := time.Duration(0)
	if tr, ok := reg.(tool.TimeoutResolver); ok {
		if d, ok2 := tr.ToolTimeout(step.Name); ok2 && d > 0 {
			registryTimeout = d
			ctx = WithToolTimeout(ctx, d)
		}
	}
	wrapperTimeout, wrapperTimeoutOK := toolTimeoutFromContext(ctx)
	argsTimeoutMs, hasArgsTimeout := timeoutMsFromArgs(step.Args)
	ctxDeadline, ctxRemaining := formatContextDeadline(ctx)
	if hasArgsTimeout {
		debugConvf("tool execute context convo=%q turn=%q op_id=%q tool=%q parent_deadline=%q parent_remaining=%q registry_timeout=%q wrapper_timeout=%q args_timeout_ms=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), ctxDeadline, ctxRemaining, registryTimeout.String(), wrapperTimeout.String(), argsTimeoutMs)
	} else if wrapperTimeoutOK {
		debugConvf("tool execute context convo=%q turn=%q op_id=%q tool=%q parent_deadline=%q parent_remaining=%q registry_timeout=%q wrapper_timeout=%q args_timeout_ms=none", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), ctxDeadline, ctxRemaining, registryTimeout.String(), wrapperTimeout.String())
	} else {
		debugConvf("tool execute context convo=%q turn=%q op_id=%q tool=%q parent_deadline=%q parent_remaining=%q registry_timeout=%q wrapper_timeout=default args_timeout_ms=none", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), ctxDeadline, ctxRemaining, registryTimeout.String())
	}
	var toolResult string
	var execErr error
	out, toolResult, execErr = executeToolWithRetry(ctx, reg, step)
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
		cause := classifyTimeoutCause(ctx, nil, execErr)
		warnConvf("tool execute error convo=%q turn=%q op_id=%q tool=%q cause=%q err=%q parent_ctx_err=%q", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(cause), strings.TrimSpace(execErr.Error()), strings.TrimSpace(formatContextErr(ctx)))
	}
	span.SetEnd(time.Now())

	// 5) Persist side effects + response payload.
	if strings.TrimSpace(toolResult) != "" {
		if err := persistDocumentsIfNeeded(ctx, reg, conv, turn, step.Name, toolResult); err != nil {
			errs = append(errs, fmt.Errorf("emit system content: %w", err))
		}
		if err := persistToolImageAttachmentIfNeeded(ctx, conv, turn, toolMsgID, step.Name, toolResult); err != nil {
			errs = append(errs, fmt.Errorf("persist tool attachments: %w", err))
		}
		if redacted, ok := redactToolResultIfNeeded(step.Name, toolResult); ok {
			toolResult = redacted
			out.Result = redacted
		}
	}

	respID, respErr := persistResponsePayload(ctx, conv, toolResult)
	if respErr != nil {
		errs = append(errs, fmt.Errorf("persist response payload: %w", respErr))
	}

	// 6) Update tool message with result content - why duplication of content gere
	//if uErr := updateToolMessageContent(persistCtx, conv, toolMsgID, toolResult); uErr != nil {
	//	errs = append(errs, fmt.Errorf("update tool message: %w", uErr))
	//}

	// 7) Finish tool call and conversation status together.
	// They are intentionally separate writes: one can fail while the other succeeds.
	// We still attempt both so terminal state is propagated to conversation-level
	// status even if tool_call persistence has partial failures (and vice versa).
	status, errMsg := resolveToolStatus(execErr, ctx)
	forcedStatus = status
	// Use detached + bounded context for terminal writes.
	finCtx, cancelFin := detachedFinalizeCtx(ctx)
	defer cancelFin()
	if cErr := completeToolCall(finCtx, conv, toolMsgID, status, span.EndedAt, respID, errMsg); cErr != nil {
		errs = append(errs, fmt.Errorf("complete tool call: %w", cErr))
	} else {
		toolCallClosed = true
	}
	patchErr := conv.PatchConversations(finCtx, convw.NewConversationStatus(turn.ConversationID, status))
	if patchErr != nil {
		errs = append(errs, fmt.Errorf("patch conversations call: %w", patchErr))
	} else {
		conversationPatched = true
	}

	if len(errs) > 0 {
		retErr = errors.Join(errs...)
	}

	if retErr != nil && len(errs) > 0 {
		errorConvf("tool execute done convo=%q turn=%q op_id=%q tool=%q status=%q result_len=%d err=%v", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(status), len(toolResult), retErr)
	} else {
		infoConvf("tool execute done convo=%q turn=%q op_id=%q tool=%q status=%q result_len=%d", strings.TrimSpace(turn.ConversationID), strings.TrimSpace(turn.TurnID), strings.TrimSpace(step.ID), strings.TrimSpace(step.Name), strings.TrimSpace(status), len(toolResult))
	}

	return
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
	finCtx, cancelFin := detachedFinalizeCtx(ctx)
	defer cancelFin()
	if cErr := completeToolCall(finCtx, conv, toolMsgID, status, completedAt, respID, ""); cErr != nil {
		return fmt.Errorf("complete tool call: %w", cErr)
	}
	_ = conv.PatchConversations(finCtx, convw.NewConversationStatus(turn.ConversationID, status))
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

func detachedFinalizeCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), finalizeWriteTimeout)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), finalizeWriteTimeout)
}

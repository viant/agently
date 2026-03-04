package executil

import (
	"context"
	"errors"
	"strings"
	"time"

	plan "github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
)

const defaultToolCallAttempts = 2

var maxRetryDuration = 5 * time.Second

// executeToolWithRetry runs a tool and retries once on transient context cancellations.
func executeToolWithRetry(ctx context.Context, reg tool.Registry, step StepInfo) (plan.ToolCall, string, error) {
	attempts := defaultToolCallAttempts
	if attempts < 1 {
		attempts = 1
	}
	var out plan.ToolCall
	var result string
	var execErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptCtx, cancel := toolExecContext(ctx)
		parentDeadline, parentRemaining := formatContextDeadline(ctx)
		attemptDeadline, attemptRemaining := formatContextDeadline(attemptCtx)
		if argsTimeoutMs, ok := timeoutMsFromArgs(step.Args); ok {
			debugConvf("tool attempt start tool=%q op_id=%q attempt=%d/%d args_timeout_ms=%d parent_deadline=%q parent_remaining=%q attempt_deadline=%q attempt_remaining=%q", strings.TrimSpace(step.Name), strings.TrimSpace(step.ID), attempt, attempts, argsTimeoutMs, parentDeadline, parentRemaining, attemptDeadline, attemptRemaining)
		} else {
			debugConvf("tool attempt start tool=%q op_id=%q attempt=%d/%d args_timeout_ms=none parent_deadline=%q parent_remaining=%q attempt_deadline=%q attempt_remaining=%q", strings.TrimSpace(step.Name), strings.TrimSpace(step.ID), attempt, attempts, parentDeadline, parentRemaining, attemptDeadline, attemptRemaining)
		}
		started := time.Now()
		out, result, execErr = executeTool(attemptCtx, reg, step)
		attemptCtxErr := attemptCtx.Err()
		cancel()
		elapsed := time.Since(started)
		if execErr != nil {
			cause := classifyTimeoutCause(ctx, attemptCtxErr, execErr)
			warnConvf("tool attempt end tool=%q op_id=%q attempt=%d/%d elapsed=%s cause=%q err=%q attempt_ctx_err=%q parent_ctx_err=%q", strings.TrimSpace(step.Name), strings.TrimSpace(step.ID), attempt, attempts, elapsed.String(), strings.TrimSpace(cause), strings.TrimSpace(execErr.Error()), strings.TrimSpace(errorString(attemptCtxErr)), strings.TrimSpace(formatContextErr(ctx)))
		} else {
			debugConvf("tool attempt end tool=%q op_id=%q attempt=%d/%d elapsed=%s status=ok", strings.TrimSpace(step.Name), strings.TrimSpace(step.ID), attempt, attempts, elapsed.String())
		}
		if execErr == nil || !shouldRetryToolCall(ctx, execErr, elapsed, attempt, attempts) {
			break
		}
		debugf(ctx, "tool %s attempt %d/%d failed after %s with %v; retrying", step.Name, attempt, attempts, elapsed, execErr)
	}
	return out, result, execErr
}

func shouldRetryToolCall(ctx context.Context, execErr error, elapsed time.Duration, attempt, maxAttempts int) bool {
	if execErr == nil {
		return false
	}
	if attempt >= maxAttempts {
		return false
	}
	if maxRetryDuration > 0 && elapsed > maxRetryDuration {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return isContextCancellationError(execErr)
}

func isContextCancellationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

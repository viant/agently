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
		started := time.Now()
		out, result, execErr = executeTool(attemptCtx, reg, step)
		cancel()
		elapsed := time.Since(started)
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

package executil

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func formatContextDeadline(ctx context.Context) (string, string) {
	if ctx == nil {
		return "none", "none"
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return "none", "none"
	}
	remaining := time.Until(deadline)
	return deadline.Format(time.RFC3339Nano), remaining.String()
}

func formatContextErr(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	return ""
}

func timeoutMsFromArgs(args map[string]interface{}) (int64, bool) {
	if args == nil {
		return 0, false
	}
	raw, ok := args["timeoutMs"]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, true
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

func classifyTimeoutCause(parentCtx context.Context, attemptCtxErr error, execErr error) string {
	if attemptCtxErr == context.DeadlineExceeded {
		return "attempt_deadline_exceeded"
	}
	if parentCtx != nil {
		switch parentCtx.Err() {
		case context.Canceled:
			return "parent_canceled"
		case context.DeadlineExceeded:
			return "parent_deadline_exceeded"
		}
	}
	if execErr != nil {
		msg := strings.ToLower(execErr.Error())
		switch {
		case strings.Contains(msg, "timed out"), strings.Contains(msg, "timeout"):
			return "tool_internal_timeout"
		case strings.Contains(msg, "context canceled"):
			return "context_canceled"
		case strings.Contains(msg, "context deadline"):
			return "context_deadline_exceeded"
		}
	}
	return ""
}

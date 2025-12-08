package executil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"strings"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	plan "github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	contpol "github.com/viant/agently/genai/tool/continuation"
	overwrap "github.com/viant/agently/genai/tool/overflow"
	schinspect "github.com/viant/agently/genai/tool/schema"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	"github.com/viant/mcp-protocol/extension"
	"strconv"
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
	execCtx, cancel := toolExecContext(ctx)
	defer cancel()

	out, toolResult, execErr := executeTool(execCtx, reg, step)
	// Optionally wrap overflow with YAML helper when native continuation is not supported.
	if wrapped := maybeWrapOverflow(execCtx, reg, step.Name, toolResult, toolMsgID); wrapped != "" {
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
	status, errMsg := resolveToolStatus(execErr, ctx)
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

// SynthesizeToolStep persists a tool call using a precomputed result without
// invoking the actual tool. It mirrors ExecuteToolStep's persistence flow
// (messages, request/response payloads, status), setting status to "completed".
func SynthesizeToolStep(ctx context.Context, conv apiconv.Client, step StepInfo, toolResult string) error {
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return fmt.Errorf("turn meta not found")
	}
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
	return nil
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
	// Persist provider trace/anchor id (response.id) for this tool call when available.
	if trace := strings.TrimSpace(traceID); trace != "" {
		tc.TraceID = &trace
		tc.Has.TraceID = true
	} else if trace := strings.TrimSpace(memory.TurnTrace(turn.TurnID)); trace != "" {
		// Fallback for legacy callers
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

// toolExecContext returns a bounded context for tool execution. It uses AGENTLY_TOOLCALL_TIMEOUT
// environment variable when set (e.g., "45s", "2m"), otherwise defaults to 60s.
func toolExecContext(ctx context.Context) (context.Context, context.CancelFunc) {
	const defaultTimeout = 3 * time.Minute
	if d, ok := toolTimeoutFromContext(ctx); ok && d > 0 {
		return context.WithTimeout(ctx, d)
	}
	if v := strings.TrimSpace(os.Getenv("AGENTLY_TOOLCALL_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return context.WithTimeout(ctx, d)
		}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// ---------------- context helpers ----------------

type ctxKey int

const (
	keyToolTimeout ctxKey = iota + 1
)

// WithToolTimeout attaches a per-tool execution timeout to the context.
func WithToolTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, keyToolTimeout, d)
}

// toolTimeoutFromContext reads a configured tool timeout from context when present.
func toolTimeoutFromContext(ctx context.Context) (time.Duration, bool) {
	if v := ctx.Value(keyToolTimeout); v != nil {
		if d, ok := v.(time.Duration); ok {
			return d, true
		}
	}
	return 0, false
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

// maybeWrapOverflow inspects a tool result for continuation hints and, based on
// input/output schemas, returns a YAML overflow wrapper when native range
// continuation is not supported. When no wrapper is needed, it returns an empty string.
func maybeWrapOverflow(ctx context.Context, reg tool.Registry, toolName, result, toolMsgID string) string {
	// Quick JSON parse to inspect continuation
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return ""
	}
	contRaw, ok := payload["continuation"].(map[string]interface{})
	if !ok || contRaw == nil {
		return ""
	}
	// Detect truncation condition: remaining > 0 or hasMore=true
	hasMore := false
	if v, ok := contRaw["hasMore"]; ok {
		if b, ok2 := v.(bool); ok2 && b {
			hasMore = true
		}
	}
	remaining := intFromAny(contRaw["remaining"]) // 0 when absent
	if !hasMore && remaining <= 0 {
		return ""
	}
	// Inspect schemas to decide strategy
	def, ok := reg.GetDefinition(toolName)
	var inShape schinspect.RangeInputs
	var outShape schinspect.ContinuationShape
	if ok && def != nil {
		_, inShape = schinspect.HasInputRanges(def.Parameters)
		_, outShape = schinspect.HasOutputContinuation(def.OutputSchema)
	}
	strat := contpol.Decide(inShape, outShape)
	switch strat {
	case contpol.OutputOnlyRanges, contpol.NoRanges:
		// Build extension.Continuation (bytes-only for now)
		ext := toContinuation(contRaw)
		yaml, err := overwrap.BuildOverflowYAML(toolMsgID, ext)
		if err == nil && strings.TrimSpace(yaml) != "" {
			return yaml
		}
		return ""
	default:
		return ""
	}
}

func intFromAny(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		if f, err := t.Float64(); err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

// toContinuation converts a generic map continuation into extension.Continuation.
func toContinuation(m map[string]interface{}) *extension.Continuation {
	if m == nil {
		return nil
	}
	c := &extension.Continuation{}
	if v, ok := m["hasMore"].(bool); ok {
		c.HasMore = v
	}
	c.Remaining = intFromAny(m["remaining"])
	c.Returned = intFromAny(m["returned"])
	// bytes nextRange when present: nextRange or nested bytes {offset,length}
	if nr, ok := m["nextRange"].(map[string]interface{}); ok && nr != nil {
		if b, ok := nr["bytes"].(map[string]interface{}); ok && b != nil {
			off := intFromAny(b["offset"])
			if off == 0 {
				off = intFromAny(b["offsetBytes"])
			}
			ln := intFromAny(b["length"])
			if ln == 0 {
				ln = intFromAny(b["lengthBytes"])
			}
			c.NextRange = &extension.RangeHint{Bytes: &extension.ByteRange{Offset: off, Length: ln}}
		}
	} else if s, ok := m["nextRange"].(string); ok && strings.Contains(s, "-") {
		// parse "X-Y" string
		parts := strings.SplitN(s, "-", 2)
		if len(parts) == 2 {
			// best effort
			// ignore parse errors → zero values
			var off, end int
			if o, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				off = o
			}
			if e, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				end = e
			}
			ln := 0
			if end > off {
				ln = end - off
			}
			c.NextRange = &extension.RangeHint{Bytes: &extension.ByteRange{Offset: off, Length: ln}}
		}
	}
	return c
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

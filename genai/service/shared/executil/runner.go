package executil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	plan "github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	domainrec "github.com/viant/agently/internal/domain/recorder"
)

// StepInfo carries the tool step data needed for execution.
type StepInfo struct {
	ID   string
	Name string
	Args map[string]interface{}
}

// RunTool executes a tool step via the registry, publishing task logs and updating traces.
// It returns the normalized plan.ToolCall, start/end timestamps and error.
func RunTool(ctx context.Context, reg tool.Registry, step StepInfo, recorder domainrec.Recorder) (plan.ToolCall, plan.CallSpan, error) {

	span := plan.CallSpan{
		StartedAt: time.Now(),
	}
	turn, ok := memory.TurnMetaFromContext(ctx)
	if !ok {
		return plan.ToolCall{}, span, fmt.Errorf("turn meta not found")
	}

	if err := recorder.StartToolCall(ctx, domainrec.ToolCallStart{
		MessageID: turn.ParentMessageID,
		TurnID:    turn.TurnID,
		ToolName:  step.Name,
		StartedAt: span.StartedAt,
		Request:   step.Args,
	}); err != nil {
		return plan.ToolCall{}, span, err
	}
	var out plan.ToolCall
	var toolResult string

	toolResult, err := reg.Execute(ctx, step.Name, step.Args)
	span.SetEnd(time.Now())
	out = plan.ToolCall{ID: step.ID, Name: step.Name, Arguments: step.Args, Result: toolResult}
	if err != nil {
		out.Error = err.Error()
	}

	// Persist a tool message so transcript captures it and link tool_call to it
	var toolMsgID string

	toolMsgID = uuid.New().String()
	name := step.Name
	msg := memory.Message{ID: toolMsgID, ConversationID: turn.ConversationID, ParentID: turn.ParentMessageID, Role: "tool", Content: out.Result, ToolName: &name, CreatedAt: span.EndedAt}
	recorder.RecordMessage(ctx, msg)

	status := "completed"
	var errMsg string
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	finishErr := recorder.FinishToolCall(ctx, domainrec.ToolCallUpdate{
		MessageID:     turn.ParentMessageID,
		ToolMessageID: toolMsgID,
		TurnID:        turn.TurnID,
		ToolName:      step.Name,
		Status:        status,
		StartedAt:     span.StartedAt,
		CompletedAt:   span.EndedAt,
		ErrMsg:        errMsg,
		Cost:          nil,
		Request:       step.Args,
		Response:      out.Result,
		OpID:          out.ID,
		Attempt:       1,
	})
	if finishErr != nil {
		if err != nil {
			return out, span, errors.Join(err, finishErr)
		}
		return out, span, finishErr
	}
	return out, span, err
}

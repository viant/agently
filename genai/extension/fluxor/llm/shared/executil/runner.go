package executil

import (
	"context"
	"time"

	"github.com/google/uuid"
	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	domainrec "github.com/viant/agently/internal/domain/recorder"
	elog "github.com/viant/agently/internal/log"
)

// Tracer abstracts execution trace updates so callers can plug their own store.
type Tracer interface {
	UpdateTraceStart(ctx context.Context, conversationID string, traceID int, startAt time.Time)
	UpdateTraceEnd(ctx context.Context, conversationID string, traceID int, result plan.Result, duplicated bool, endAt time.Time)
}

// StepInfo carries the tool step data needed for execution.
type StepInfo struct {
	ID   string
	Name string
	Args map[string]interface{}
}

// Options configures RunTool behaviour.
type Options struct {
	Tracer           Tracer
	ConversationID   string
	TraceID          int
	Duplicated       bool
	DuplicatedResult *plan.Result
	Recorder         domainrec.Recorder
}

// Option configures Options via functional options pattern.
type Option func(*Options)

// WithTracer sets a tracer.
func WithTracer(t Tracer) Option { return func(o *Options) { o.Tracer = t } }

// WithConversation sets conversation ID context for tracing.
func WithConversation(id string) Option { return func(o *Options) { o.ConversationID = id } }

// WithTrace sets the trace ID.
func WithTrace(id int) Option { return func(o *Options) { o.TraceID = id } }

// WithDuplicated flags the call as duplicated.
func WithDuplicated(dup bool) Option { return func(o *Options) { o.Duplicated = dup } }

// WithDuplicatedResult provides a precomputed result for duplicated calls.
func WithDuplicatedResult(r *plan.Result) Option { return func(o *Options) { o.DuplicatedResult = r } }

// WithRecorder sets a domain recorder for tool-call persistence.
func WithRecorder(rec domainrec.Recorder) Option {
	return func(o *Options) { o.Recorder = rec }
}

// RunTool executes a tool step via the registry, publishing task logs and updating traces.
// It returns the normalized plan.Result, start/end timestamps and error.
func RunTool(ctx context.Context, reg tool.Registry, step StepInfo, optFns ...Option) (plan.Result, time.Time, time.Time, error) {
	// build options
	opts := Options{}
	for _, fn := range optFns {
		if fn != nil {
			fn(&opts)
		}
	}
	// Emit TaskInput
	elog.Publish(elog.Event{Time: time.Now(), EventType: elog.TaskInput, Payload: map[string]interface{}{"tool": step.Name, "args": step.Args}})

	startedAt := time.Now()
	if opts.Tracer != nil {
		opts.Tracer.UpdateTraceStart(ctx, opts.ConversationID, opts.TraceID, startedAt)
	}

	var err error
	///
	// Domain recorder – best-effort capture of tool call start
	if opts.Recorder != nil {
		msgID := memory.MessageIDFromContext(ctx)
		turnID := memory.TurnIDFromContext(ctx)
		opts.Recorder.StartToolCall(ctx, domainrec.ToolCallStart{
			MessageID: msgID,
			TurnID:    turnID,
			ToolName:  step.Name,
			StartedAt: startedAt,
			Request:   step.Args,
		})
	}

	var out plan.Result
	var toolResult string

	if opts.Duplicated && opts.DuplicatedResult != nil {
		// Short-circuit using the provided duplicated result
		out = *opts.DuplicatedResult
	} else {
		toolResult, err = reg.Execute(ctx, step.Name, step.Args)
		out = plan.Result{ID: step.ID, Name: step.Name, Args: step.Args, Result: toolResult}
		if err != nil {
			out.Error = err.Error()
		}
	}

	endedAt := time.Now()

	// Emit TaskOutput
	payload := map[string]interface{}{"tool": step.Name, "result": out.Result, "error": err}
	elog.Publish(elog.Event{Time: time.Now(), EventType: elog.TaskOutput, Payload: payload})

	if opts.Tracer != nil {
		opts.Tracer.UpdateTraceEnd(ctx, opts.ConversationID, opts.TraceID, out, opts.Duplicated, endedAt)
	}

	// Persist a tool message so transcript captures it and link tool_call to it
	var toolMsgID string
	if convID := memory.ConversationIDFromContext(ctx); convID != "" && opts.Recorder != nil {
		toolMsgID = uuid.New().String()
		parent := memory.MessageIDFromContext(ctx)
		name := step.Name
		msg := memory.Message{ID: toolMsgID, ConversationID: convID, ParentID: parent, Role: "tool", Content: out.Result, ToolName: &name, CreatedAt: endedAt}
		opts.Recorder.RecordMessage(ctx, msg)
	}

	// Domain recorder – best-effort capture of tool call completion
	if opts.Recorder != nil {
		msgID := memory.MessageIDFromContext(ctx)
		turnID := memory.TurnIDFromContext(ctx)
		status := "completed"
		var errMsg string
		if err != nil {
			status = "failed"
			errMsg = err.Error()
		}
		opts.Recorder.FinishToolCall(ctx, domainrec.ToolCallUpdate{
			MessageID:     msgID,
			ToolMessageID: toolMsgID,
			TurnID:        turnID,
			ToolName:      step.Name,
			Status:        status,
			StartedAt:     startedAt,
			CompletedAt:   endedAt,
			ErrMsg:        errMsg,
			Cost:          nil,
			Request:       step.Args,
			Response:      out.Result,
			OpID:          out.ID,
			Attempt:       1,
		})
	}
	return out, startedAt, endedAt, err
}

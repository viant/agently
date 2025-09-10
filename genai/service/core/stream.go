package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"time"

	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/modelcallctx"
	stream2 "github.com/viant/agently/genai/service/core/stream"
	fluxortypes "github.com/viant/fluxor/model/types"
)

type StreamInput struct {
	*GenerateInput
	StreamID string
}

// StreamOutput aggregates streaming events into a slice.
type StreamOutput struct {
	Events    []stream2.Event `json:"events"`
	MessageID string          `json:"messageId,omitempty"`
}

// Stream handles streaming LLM responses, structuring JSON output for text chunks,
// function calls and finish reasons.
func (s *Service) Stream(ctx context.Context, in, out interface{}) error {
	input, output, err := s.validateStreamIO(in, out)
	if err != nil {
		return err
	}
	handler, cleanup, err := stream2.PrepareStreamHandler(ctx, input.StreamID)
	if err != nil {
		return err
	}
	defer cleanup()

	s.ensureStreamingOption(input)
	req, model, err := s.prepareGenerateRequest(ctx, input.GenerateInput)
	if err != nil {
		return err
	}
	streamer, ok := model.(llm.StreamingModel)
	if !ok {
		return fmt.Errorf("model %T does not support streaming", model)
	}
	ctx = modelcallctx.WithRecorderObserver(ctx, s.recorder)

	streamCh, err := streamer.Stream(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start Stream: %w", err)
	}
	if err := s.consumeEvents(ctx, streamCh, handler, output); err != nil {
		return err
	}

	var b strings.Builder
	// keep for completeness
	for _, ev := range output.Events {
		if ev.Type == "chunk" && strings.TrimSpace(ev.Content) != "" {
			b.WriteString(ev.Content)
		}
		// function_call events detection is handled in the model-call observer.
	}
	content := strings.TrimSpace(b.String())
	if content != "" {
		msgID := memory.ModelMessageIDFromContext(ctx)
		turn, _ := memory.TurnMetaFromContext(ctx)
		if turn.ConversationID != "" {
			s.recorder.RecordMessage(ctx, memory.Message{ID: msgID, ParentID: turn.ParentMessageID, ConversationID: turn.ConversationID, Role: "assistant", Content: content, CreatedAt: time.Now()})
			// Marking interim planner is now handled in the model-call observer based on final response.
			output.MessageID = msgID
		}
	}

	return nil
}

// validateStreamIO validates and unwraps inputs.
func (s *Service) validateStreamIO(in, out interface{}) (*StreamInput, *StreamOutput, error) {
	input, ok := in.(*StreamInput)
	if !ok {
		return nil, nil, fluxortypes.NewInvalidInputError(in)
	}
	output, ok := out.(*StreamOutput)
	if !ok {
		return nil, nil, fluxortypes.NewInvalidOutputError(out)
	}
	if input.StreamID == "" {
		return nil, nil, fmt.Errorf("streamID was empty")
	}
	return input, output, nil
}

// ensureStreamingOption turns on streaming at the request level.
func (s *Service) ensureStreamingOption(input *StreamInput) {
	if input.Options == nil {
		input.Options = &llm.Options{}
	}
	input.Options.Stream = true
}

// consumeEvents pulls from provider Stream channel, dispatches to handler and
// appends structured events to output. Stops on error or done.
func (s *Service) consumeEvents(ctx context.Context, ch <-chan llm.StreamEvent, handler stream2.Handler, output *StreamOutput) error {
	for event := range ch {
		if err := handler(ctx, &event); err != nil {
			return fmt.Errorf("failed to handle Stream event: %w", err)
		}
		if err := s.appendStreamEvent(&event, output); err != nil {
			return err
		}
		// Stop on done or error
		if len(output.Events) > 0 {
			last := output.Events[len(output.Events)-1]
			if last.Type == "done" || last.Type == "error" {
				break
			}
		}
	}
	return nil
}

// appendStreamEvent converts provider event to public stream.Event(s).
func (s *Service) appendStreamEvent(event *llm.StreamEvent, output *StreamOutput) error {
	if event.Err != nil {
		output.Events = append(output.Events, stream2.Event{Type: "error", Content: event.Err.Error()})
		return event.Err
	}
	resp := event.Response
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	choice := resp.Choices[0]
	// Tool calls
	if len(choice.Message.ToolCalls) > 0 {
		output.Events = append(output.Events, s.toolCallEvents(&choice)...)
	}
	// Text chunk
	if content := strings.TrimSpace(choice.Message.Content); content != "" {
		output.Events = append(output.Events, stream2.Event{Type: "chunk", Content: content})
	}
	// Done
	if choice.FinishReason != "" {
		output.Events = append(output.Events, stream2.Event{Type: "done", FinishReason: choice.FinishReason})
	}
	return nil
}

func (s *Service) toolCallEvents(choice *llm.Choice) []stream2.Event {
	out := make([]stream2.Event, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		name := tc.Name
		args := tc.Arguments
		if name == "" && tc.Function.Name != "" {
			name = tc.Function.Name
		}
		if args == nil && tc.Function.Arguments != "" {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err == nil {
				args = parsed
			}
		}
		out = append(out, stream2.Event{ID: tc.ID, Type: "function_call", Name: name, Arguments: args})
	}
	return out
}

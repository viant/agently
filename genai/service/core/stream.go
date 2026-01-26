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
	stream "github.com/viant/agently/genai/service/core/stream"
	svc "github.com/viant/agently/genai/tool/service"
)

type StreamInput struct {
	*GenerateInput
	StreamID string
}

// StreamOutput aggregates streaming events into a slice.
type StreamOutput struct {
	Events    []stream.Event `json:"events"`
	MessageID string         `json:"messageId,omitempty"`
}

func nop() {}

// Stream handles streaming LLM responses, structuring JSON output for text chunks,
// function calls and finish reasons.
func (s *Service) Stream(ctx context.Context, in, out interface{}) (func(), error) {
	input, output, err := s.validateStreamIO(in, out)
	if err != nil {
		return nop, err
	}
	handler, cleanup, err := stream.PrepareStreamHandler(ctx, input.StreamID)
	if err != nil {
		return nop, err
	}

	s.ensureStreamingOption(input)
	req, model, err := s.prepareGenerateRequest(ctx, input.GenerateInput)
	if err != nil {
		return cleanup, err
	}
	streamer, ok := model.(llm.StreamingModel)
	if !ok {
		return cleanup, fmt.Errorf("model %T does not support streaming", model)
	}
	// Attach finish barrier so final message waits for model-call persistence.
	ctx, _ = modelcallctx.WithFinishBarrier(ctx)
	if s.streamPub != nil {
		ctx = modelcallctx.WithStreamPublisher(ctx, s.streamPub)
	}
	// Inject recorder with price resolver when available so cost gets computed.
	if tp, ok := s.llmFinder.(modelcallctx.TokenPriceProvider); ok {
		declared := ""
		if input != nil && input.GenerateInput != nil {
			declared = strings.TrimSpace(input.GenerateInput.Model)
		}
		if declared != "" {
			tp = modelcallctx.NewFixedModelPriceProvider(tp, declared)
		}
		ctx = modelcallctx.WithRecorderObserverWithPrice(ctx, s.convClient, tp)
	} else {
		ctx = modelcallctx.WithRecorderObserver(ctx, s.convClient)
	}

	var continuationRequest *llm.GenerateRequest
	if IsContextContinuationEnabled(model) {
		continuationRequest = s.BuildContinuationRequest(ctx, req, &input.GenerateInput.Binding.History)

	}

	// Retry starting stream up to 3 attempts. Consult provider-specific
	// BackoffAdvisor (e.g., Bedrock ThrottlingException -> 30s) when available.
	var streamCh <-chan llm.StreamEvent
	for attempt := 0; attempt < 3; attempt++ {
		llmRequest := req
		if continuationRequest != nil {
			llmRequest = continuationRequest
		}
		streamCh, err = streamer.Stream(ctx, llmRequest)
		if err == nil {
			break
		}

		if isContextLimitError(err) {
			return cleanup, fmt.Errorf("%w: %v", ErrContextLimitExceeded, err)
		}
		if advisor, ok := model.(llm.BackoffAdvisor); ok {
			if delay, retry := advisor.AdviseBackoff(err, attempt); retry {
				if attempt == 2 || ctx.Err() != nil {
					return cleanup, fmt.Errorf("failed to start Stream: %w", err)
				}
				// Set model_call status to retrying before waiting
				s.setModelCallStatus(ctx, "retrying")
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return cleanup, fmt.Errorf("failed to start Stream: %w", err)
				}
				continue
			}
		}
		if !isTransientNetworkError(err) || attempt == 2 || ctx.Err() != nil {
			return cleanup, fmt.Errorf("failed to start Stream: %w", err)
		}
		// Backoff: 1s, 2s, 4s
		delay := time.Second << attempt
		// Set model_call status to retrying before waiting
		s.setModelCallStatus(ctx, "retrying")
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return cleanup, fmt.Errorf("failed to start Stream: %w", err)
		}
	}
	if err := s.consumeEvents(ctx, streamCh, handler, output); err != nil {
		return cleanup, err
	}

	var b strings.Builder
	// keep for completeness
	for _, ev := range output.Events {
		if ev.Type == "chunk" && strings.TrimSpace(ev.Content) != "" {
			b.WriteString(ev.Content)
		}
	}

	// Provide the shared assistant message ID to the caller; orchestrator writes the final assistant message.
	if msgID := memory.ModelMessageIDFromContext(ctx); msgID != "" {
		output.MessageID = msgID
	}

	return cleanup, nil
}

// moved to continuation.go

// validateStreamIO validates and unwraps inputs.
func (s *Service) validateStreamIO(in, out interface{}) (*StreamInput, *StreamOutput, error) {
	input, ok := in.(*StreamInput)
	if !ok {
		return nil, nil, svc.NewInvalidInputError(in)
	}
	output, ok := out.(*StreamOutput)
	if !ok {
		return nil, nil, svc.NewInvalidOutputError(out)
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
func (s *Service) consumeEvents(ctx context.Context, ch <-chan llm.StreamEvent, handler stream.Handler, output *StreamOutput) error {
	var resErr error
	var ignore bool

	for event := range ch {
		if ignore {
			continue
		}

		if err := handler(ctx, &event); err != nil {
			// If the handler surfaced a provider error that indicates a context/window overflow,
			// wrap it with ErrContextLimitExceeded so callers can reliably detect it via errors.Is.
			if isContextLimitError(err) {
				// Preserve the human-friendly prefix while keeping the sentinel in the wrap chain.
				resErr = fmt.Errorf("failed to handle Stream event: %w", fmt.Errorf("%w: %v", ErrContextLimitExceeded, err))
			} else {
				resErr = fmt.Errorf("failed to handle Stream event: %w", err)
			}
			ignore = true
			continue
		}

		if err := s.appendStreamEvent(&event, output); err != nil {
			resErr = err
			ignore = true
			continue
		}

		// Stop on done or error
		if len(output.Events) > 0 {
			last := output.Events[len(output.Events)-1]
			// For tool-calls, finish_reason == "tool_calls" indicates that the
			// model is requesting tools, not that the overall stream is done.
			// In that case we must continue consuming subsequent events so that
			// additional tool_call items (and the final assistant message) are
			// observed and executed.
			if last.Type == "done" {
				if strings.TrimSpace(last.FinishReason) != "tool_calls" {
					ignore = true
				}
			} else if last.Type == "error" {
				ignore = true
			}
		}
	}

	return resErr
}

// appendStreamEvent converts provider event to public stream.Event(s).
func (s *Service) appendStreamEvent(event *llm.StreamEvent, output *StreamOutput) error {
	if event.Err != nil {
		output.Events = append(output.Events, stream.Event{Type: "error", Content: event.Err.Error()})
		if isContextLimitError(event.Err) {
			return fmt.Errorf("%w: %v", ErrContextLimitExceeded, event.Err)
		}
		return event.Err
	}
	resp := event.Response
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	choice := resp.Choices[0]
	// Tool calls
	if len(choice.Message.ToolCalls) > 0 {
		output.Events = append(output.Events, s.toolCallEvents(resp.ResponseID, &choice)...)
	}
	// Text chunk
	if content := strings.TrimSpace(choice.Message.Content); content != "" {
		output.Events = append(output.Events, stream.Event{Type: "chunk", Content: content})
	}
	// Done
	if choice.FinishReason != "" {
		output.Events = append(output.Events, stream.Event{Type: "done", FinishReason: choice.FinishReason})
	}
	return nil
}

func (s *Service) toolCallEvents(responseID string, choice *llm.Choice) []stream.Event {
	out := make([]stream.Event, 0, len(choice.Message.ToolCalls))
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
		out = append(out, stream.Event{ID: tc.ID, Type: "function_call", Name: name, Arguments: args, ResponseID: strings.TrimSpace(responseID)})
	}
	return out
}

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/viant/agently/genai/llm"
	fluxortypes "github.com/viant/fluxor/model/types"
)

// StreamEvent represents a partial or complete event in a streaming LLM response.
// It captures text chunks, function calls, and final finish reasons.
type StreamEvent struct {
	Type         string                 `json:"type"`                   // chunk, function_call or done or error
	Content      string                 `json:"content,omitempty"`      // text content for chunk events
	Name         string                 `json:"name,omitempty"`         // function name for function_call events
	Arguments    map[string]interface{} `json:"arguments,omitempty"`    // function arguments for function_call events
	FinishReason string                 `json:"finishReason,omitempty"` // finish_reason for done events
}

// StreamOutput aggregates streaming events into a slice.
type StreamOutput struct {
	Events []StreamEvent `json:"events"`
}

// stream handles streaming LLM responses, structuring JSON output for text chunks,
// function calls and finish reasons.
func (s *Service) stream(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*GenerateInput)
	if !ok {
		return fluxortypes.NewInvalidInputError(in)
	}
	output, ok := out.(*StreamOutput)
	if !ok {
		return fluxortypes.NewInvalidOutputError(out)
	}

	// Enable streaming
	if input.Options == nil {
		input.Options = &llm.Options{}
	}
	input.Options.Stream = true

	req, model, err := s.prepareGenerateRequest(ctx, input)
	if err != nil {
		return err
	}

	streamer, ok := model.(llm.StreamingModel)
	if !ok {
		return fmt.Errorf("model %T does not support streaming", model)
	}
	// Start streaming
	streamCh, err := streamer.Stream(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	for event := range streamCh {
		if event.Err != nil {
			output.Events = append(output.Events, StreamEvent{Type: "error", Content: event.Err.Error()})
			return event.Err
		}
		resp := event.Response
		if resp == nil || len(resp.Choices) == 0 {
			continue
		}
		choice := resp.Choices[0]

		// Function call events
		if len(choice.Message.ToolCalls) > 0 {
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
				output.Events = append(output.Events, StreamEvent{Type: "function_call", Name: name, Arguments: args})
			}
		}

		// Text chunk
		if content := strings.TrimSpace(choice.Message.Content); content != "" {
			output.Events = append(output.Events, StreamEvent{Type: "chunk", Content: content})
		}

		// Finish event
		if choice.FinishReason != "" {
			output.Events = append(output.Events, StreamEvent{Type: "done", FinishReason: choice.FinishReason})
			break
		}
	}
	return nil
}

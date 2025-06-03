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

	// Expand prompt template if needed
	if input.Prompt == "" && input.Template != "" {
		expanded, err := s.expandTemplate(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to expand template: %w", err)
		}
		input.Prompt = expanded
	}

	// Pick default model if missing
	if input.Model == "" {
		if input.Preferences != nil {
			if model := s.modelMatcher.Best(input.Preferences); model != "" {
				input.Model = model
			}
		}
		if input.Model == "" {
			input.Model = s.defaultModel
		}
	}

	input.Init(ctx)
	if err := input.Validate(ctx); err != nil {
		return err
	}

	model, err := s.llmFinder.Find(ctx, input.Model)
	if err != nil {
		return fmt.Errorf("failed to find model: %w", err)
	}

	// Build messages (reuse logic from generate)
	messages := make([]llm.Message, 0, len(input.Message))
	for _, msg := range input.Message {
		items := make([]llm.ContentItem, 0)
		for i := range msg.Content {
			var item llm.ContentItem
			switch {
			case strings.Contains(fmt.Sprintf("%T", msg.Content[i]), "Text"):
				item = llm.ContentItem{Type: llm.ContentTypeText, Source: llm.SourceRaw, Data: fmt.Sprintf("%v", msg.Content[i])}
			case strings.Contains(fmt.Sprintf("%T", msg.Content[i]), "Image"):
				item = llm.ContentItem{Type: llm.ContentTypeImage, Source: llm.SourceURL, Data: fmt.Sprintf("%v", msg.Content[i])}
			case strings.Contains(fmt.Sprintf("%T", msg.Content[i]), "Binary"):
				item = llm.ContentItem{Type: llm.ContentTypeBinary, Source: llm.SourceRaw, Data: fmt.Sprintf("%v", msg.Content[i])}
			default:
				continue
			}
			items = append(items, item)
		}

		var role llm.MessageRole
		roleStr := fmt.Sprintf("%v", msg.Role)
		switch {
		case strings.Contains(roleStr, "Human"):
			role = llm.RoleUser
		case strings.Contains(roleStr, "AI"):
			role = llm.RoleAssistant
		case strings.Contains(roleStr, "System"):
			role = llm.RoleSystem
		case strings.Contains(roleStr, "Tool"):
			role = llm.RoleTool
		default:
			role = llm.RoleUser
		}

		messages = append(messages, llm.Message{Role: role, Items: items})
	}

	// Prepare options clone
	var opts *llm.Options
	if input.Options != nil {
		clone := *input.Options
		opts = &clone
	} else {
		opts = &llm.Options{}
	}
	if len(opts.Tools) == 0 && len(input.Tools) > 0 {
		opts.Tools = input.Tools
	}

	request := &llm.GenerateRequest{Messages: messages, Options: opts}

	streamer, ok := model.(llm.StreamingModel)
	if !ok {
		return fmt.Errorf("model %T does not support streaming", model)
	}
	// Start streaming
	streamCh, err := streamer.Stream(ctx, request)
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

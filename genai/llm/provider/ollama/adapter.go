package ollama

import (
	"context"
	"github.com/viant/agently/genai/llm"
	"strings"
)

// ToRequest converts an llm.ChatRequest to an Ollama API Request
func ToRequest(ctx context.Context, request *llm.GenerateRequest, model string) (*Request, error) {
	req := &Request{
		Model:  model,
		Stream: false,
		Format: "json",
	}

	// Set options and streaming flag if provided
	if request.Options != nil {
		req.Stream = request.Options.Stream
		req.Options = &Options{
			Temperature:   request.Options.Temperature,
			TopP:          request.Options.TopP,
			NumPredict:    request.Options.MaxTokens,
			RepeatPenalty: 1.1, // Default value
		}

		// Add stop sequences if provided
		if len(request.Options.StopWords) > 0 {
			req.Options.Stop = request.Options.StopWords
		} else {
			// Default stop sequences
			req.Options.Stop = []string{"Human:", "User:"}
		}
	}

	// Find system message
	for _, msg := range request.Messages {
		if msg.Role == llm.RoleSystem {
			req.System = llm.MessageText(msg)
			break
		}
	}
	if strings.TrimSpace(request.Instructions) != "" {
		req.System = strings.TrimSpace(request.Instructions)
	}

	// Construct prompt from user and assistant messages
	var prompt string
	for _, msg := range request.Messages {
		// Skip system messages as they're handled separately
		if msg.Role == llm.RoleSystem {
			continue
		}

		// Add role prefix
		switch msg.Role {
		case llm.RoleUser:
			userContent := msg.Content
			if strings.TrimSpace(msg.Name) != "" {
				userContent = msg.Name + ":" + userContent
			}
			prompt += "Human: " + userContent + "\n"
		case llm.RoleAssistant:
			prompt += "Assistant: " + msg.Content + "\n"
		}
	}

	// Add final assistant prompt
	prompt += "Assistant: "
	req.Prompt = prompt

	return req, nil
}

// ToLLMSResponse converts an Ollama API Response to an llm.ChatResponse
func ToLLMSResponse(resp *Response) *llm.GenerateResponse {
	// Create the response
	return &llm.GenerateResponse{
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: resp.Response,
				},
				FinishReason: "stop",
			},
		},
		Usage: &llm.Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
			ContextTokens:    resp.Context,
		},
		Model: resp.Model,
	}
}

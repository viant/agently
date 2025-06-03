package ollama

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestToRequest_StreamFlag(t *testing.T) {
	testCases := []struct {
		description string
		input       llm.GenerateRequest
		expected    *Request
	}{
		{
			description: "streaming enabled",
			input: llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewUserMessage("Hello world"),
				},
				Options: &llm.Options{
					Temperature: 0.7,
					TopP:        0.8,
					MaxTokens:   5,
					Stream:      true,
				},
			},
			expected: &Request{
				Model:  "test-model",
				Stream: true,
				Format: "json",
				Options: &Options{
					Temperature:   0.7,
					TopP:          0.8,
					NumPredict:    5,
					RepeatPenalty: 1.1,
					Stop:          []string{"Human:", "User:"},
				},
				Prompt: "Human: Hello world\nAssistant: ",
			},
		},
		{
			description: "streaming disabled",
			input: llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewUserMessage("Test"),
				},
				Options: &llm.Options{Stream: false},
			},
			expected: &Request{
				Model:  "test-model",
				Stream: false,
				Format: "json",
				Options: &Options{
					Temperature:   0,
					TopP:          0,
					NumPredict:    0,
					RepeatPenalty: 1.1,
					Stop:          []string{"Human:", "User:"},
				},
				Prompt: "Human: Test\nAssistant: ",
			},
		},
		{
			description: "custom stop words",
			input: llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewUserMessage("Hi"),
				},
				Options: &llm.Options{
					Stream:    true,
					StopWords: []string{"Foo"},
				},
			},
			expected: &Request{
				Model:  "test-model",
				Stream: true,
				Format: "json",
				Options: &Options{
					Temperature:   0,
					TopP:          0,
					NumPredict:    0,
					RepeatPenalty: 1.1,
					Stop:          []string{"Foo"},
				},
				Prompt: "Human: Hi\nAssistant: ",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual, err := ToRequest(context.Background(), &tc.input, "test-model")
			if !assert.NoError(t, err) {
				return
			}
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}

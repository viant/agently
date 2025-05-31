//go:build integration
// +build integration

package gemini

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"testing"
)

func TestToRequest(t *testing.T) {
	testCases := []struct {
		description string
		input       *llm.GenerateRequest
		expected    *Request
	}{
		{
			description: "simple chat request",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewSystemMessage("You are a helpful assistant."),
					llm.NewUserMessage("Hello, how are you?"),
				},
				Options: &llm.Options{
					Temperature: 0.7,
					MaxTokens:   50,
				},
			},
			expected: &Request{
				Contents: []Content{
					{
						Role: "system",
						Parts: []Part{
							{
								Text: "You are a helpful assistant.",
							},
						},
					},
					{
						Role: "user",
						Parts: []Part{
							{
								Text: "Hello, how are you?",
							},
						},
					},
				},
				GenerationConfig: &GenerationConfig{
					Temperature:     0.7,
					MaxOutputTokens: 50,
				},
			},
		},
		{
			description: "chat request with file data",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					{
						Role: llm.RoleUser,
						Items: []llm.ContentItem{
							{
								Type:   llm.ContentTypeText,
								Source: llm.SourceRaw,
								Data:   "What's in this image?",
							},
							{
								Type:   llm.ContentTypeImage,
								Source: llm.SourceURL,
								Data:   "file:///path/to/image.jpg",
							},
						},
					},
				},
				Options: &llm.Options{
					Temperature: 0.7,
					MaxTokens:   50,
				},
			},
			expected: &Request{
				Contents: []Content{
					{
						Role: "user",
						Parts: []Part{
							{
								Text: "What's in this image?",
							},
							{
								FileData: &FileData{
									MimeType: "image/jpeg",
									FileURI:  "file:///path/to/image.jpg",
								},
							},
						},
					},
				},
				GenerationConfig: &GenerationConfig{
					Temperature:     0.7,
					MaxOutputTokens: 50,
				},
			},
		},
		{
			description: "chat request with video metadata",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					{
						Role: llm.RoleUser,
						Items: []llm.ContentItem{
							{
								Type:   llm.ContentTypeText,
								Source: llm.SourceRaw,
								Data:   "What's happening in this video?",
							},
							{
								Type:   llm.ContentTypeVideo,
								Source: llm.SourceURL,
								Data:   "https://example.com/video.mp4",
								Metadata: map[string]interface{}{
									"startSeconds": 10,
									"startNanos":   500000000,
									"endSeconds":   20,
									"endNanos":     0,
								},
							},
						},
					},
				},
				Options: &llm.Options{
					Temperature: 0.7,
					MaxTokens:   50,
				},
			},
			expected: &Request{
				Contents: []Content{
					{
						Role: "user",
						Parts: []Part{
							{
								Text: "What's happening in this video?",
							},
							{
								InlineData: &InlineData{
									MimeType: "video/mp4",
									Data:     "https://example.com/video.mp4",
								},
								VideoMetadata: &VideoMetadata{
									StartOffset: &Offset{
										Seconds: 10,
										Nanos:   500000000,
									},
									EndOffset: &Offset{
										Seconds: 20,
										Nanos:   0,
									},
								},
							},
						},
					},
				},
				GenerationConfig: &GenerationConfig{
					Temperature:     0.7,
					MaxOutputTokens: 50,
				},
			},
		},
		{
			description: "chat request with tools",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					llm.NewUserMessage("What's the weather in New York?"),
				},
				Options: &llm.Options{
					Tools: []llm.Tool{
						{
							Type: "function",
							Definition: llm.ToolDefinition{
								Name:        "get_weather",
								Description: "Get the weather in a location",
								Parameters: map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"location": map[string]interface{}{
											"type":        "string",
											"description": "The location to get weather for",
										},
									},
									"required": []string{"location"},
								},
							},
						},
					},
					ToolChoice: llm.NewAutoToolChoice(),
				},
			},
			expected: &Request{
				Contents: []Content{
					{
						Role: "user",
						Parts: []Part{
							{
								Text: "What's the weather in New York?",
							},
						},
					},
				},
				Tools: []Tool{
					{
						FunctionDeclarations: []FunctionDeclaration{
							{
								Name:        "get_weather",
								Description: "Get the weather in a location",
								Parameters: map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"location": map[string]interface{}{
											"type":        "string",
											"description": "The location to get weather for",
										},
									},
									"required": []string{"location"},
								},
							},
						},
					},
				},
				ToolConfig: &ToolConfig{
					FunctionCallingConfig: &FunctionCallingConfig{
						Mode: "AUTO",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual, _ := ToRequest(context.Background(), tc.input)

			// System message is now part of Contents, so no need to verify separately

			// Validate entire actual response matches expected using deep equality,
			// per project testing guidelines.
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}

func TestToLLMSResponse(t *testing.T) {
	testCases := []struct {
		description string
		input       *Response
		expected    *llm.GenerateResponse
	}{
		{
			description: "simple response",
			input: &Response{
				Candidates: []Candidate{
					{
						Content: Content{
							Role: "model",
							Parts: []Part{
								{
									Text: "I'm doing well, thank you for asking!",
								},
							},
						},
						FinishReason: "STOP",
						Index:        0,
					},
				},
				UsageMetadata: &UsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 15,
					TotalTokenCount:      25,
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "I'm doing well, thank you for asking!",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "I'm doing well, thank you for asking!",
									Text:   "I'm doing well, thank you for asking!",
								},
							},
							ContentItems: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "I'm doing well, thank you for asking!",
									Text:   "I'm doing well, thank you for asking!",
								},
							},
						},
						FinishReason: "STOP",
						Index:        0,
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     10,
					CompletionTokens: 15,
					TotalTokens:      25,
				},
			},
		},
		{
			description: "response with citations",
			input: &Response{
				Candidates: []Candidate{
					{
						Content: Content{
							Role: "model",
							Parts: []Part{
								{
									Text: "According to research, climate change is a significant issue.",
								},
							},
						},
						FinishReason: "STOP",
						Index:        0,
						CitationMetadata: &CitationMetadata{
							Citations: []Citation{
								{
									StartIndex: 0,
									EndIndex:   54,
									URI:        "https://example.com/climate-research",
									Title:      "Climate Research Paper",
									License:    "CC BY 4.0",
									PublicationDate: &Date{
										Year:  2023,
										Month: 6,
										Day:   15,
									},
								},
							},
						},
					},
				},
				UsageMetadata: &UsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 15,
					TotalTokenCount:      25,
				},
				ModelVersion: "gemini-2.0-flash-001",
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "According to research, climate change is a significant issue.",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "According to research, climate change is a significant issue.",
									Text:   "According to research, climate change is a significant issue.",
									Metadata: map[string]interface{}{
										"citations": []Citation{
											{
												StartIndex: 0,
												EndIndex:   54,
												URI:        "https://example.com/climate-research",
												Title:      "Climate Research Paper",
												License:    "CC BY 4.0",
												PublicationDate: &Date{
													Year:  2023,
													Month: 6,
													Day:   15,
												},
											},
										},
										"modelVersion": "gemini-2.0-flash-001",
									},
								},
							},
							ContentItems: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "According to research, climate change is a significant issue.",
									Text:   "According to research, climate change is a significant issue.",
									Metadata: map[string]interface{}{
										"citations": []Citation{
											{
												StartIndex: 0,
												EndIndex:   54,
												URI:        "https://example.com/climate-research",
												Title:      "Climate Research Paper",
												License:    "CC BY 4.0",
												PublicationDate: &Date{
													Year:  2023,
													Month: 6,
													Day:   15,
												},
											},
										},
										"modelVersion": "gemini-2.0-flash-001",
									},
								},
							},
						},
						FinishReason: "STOP",
						Index:        0,
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     10,
					CompletionTokens: 15,
					TotalTokens:      25,
				},
			},
		},
		{
			description: "response with logprobs",
			input: &Response{
				Candidates: []Candidate{
					{
						Content: Content{
							Role: "model",
							Parts: []Part{
								{
									Text: "Hello world",
								},
							},
						},
						FinishReason: "STOP",
						Index:        0,
						AvgLogprobs:  0.95,
						LogprobsResult: &LogprobsResult{
							TopCandidates: []TokenCandidates{
								{
									Candidates: []TokenLogprob{
										{
											Token:          "Hello",
											LogProbability: -0.05,
										},
									},
								},
								{
									Candidates: []TokenLogprob{
										{
											Token:          " world",
											LogProbability: -0.1,
										},
									},
								},
							},
							ChosenCandidates: []TokenLogprob{
								{
									Token:          "Hello",
									LogProbability: -0.05,
								},
								{
									Token:          " world",
									LogProbability: -0.1,
								},
							},
						},
					},
				},
				UsageMetadata: &UsageMetadata{
					PromptTokenCount:     5,
					CandidatesTokenCount: 2,
					TotalTokenCount:      7,
				},
			},
			expected: &llm.GenerateResponse{
				Choices: []llm.Choice{
					{
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: "Hello world",
							Items: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "Hello world",
									Text:   "Hello world",
									Metadata: map[string]interface{}{
										"avgLogprobs": 0.95,
										"logprobs": &LogprobsResult{
											TopCandidates: []TokenCandidates{
												{
													Candidates: []TokenLogprob{
														{
															Token:          "Hello",
															LogProbability: -0.05,
														},
													},
												},
												{
													Candidates: []TokenLogprob{
														{
															Token:          " world",
															LogProbability: -0.1,
														},
													},
												},
											},
											ChosenCandidates: []TokenLogprob{
												{
													Token:          "Hello",
													LogProbability: -0.05,
												},
												{
													Token:          " world",
													LogProbability: -0.1,
												},
											},
										},
									},
								},
							},
							ContentItems: []llm.ContentItem{
								{
									Type:   llm.ContentTypeText,
									Source: llm.SourceRaw,
									Data:   "Hello world",
									Text:   "Hello world",
									Metadata: map[string]interface{}{
										"avgLogprobs": 0.95,
										"logprobs": &LogprobsResult{
											TopCandidates: []TokenCandidates{
												{
													Candidates: []TokenLogprob{
														{
															Token:          "Hello",
															LogProbability: -0.05,
														},
													},
												},
												{
													Candidates: []TokenLogprob{
														{
															Token:          " world",
															LogProbability: -0.1,
														},
													},
												},
											},
											ChosenCandidates: []TokenLogprob{
												{
													Token:          "Hello",
													LogProbability: -0.05,
												},
												{
													Token:          " world",
													LogProbability: -0.1,
												},
											},
										},
									},
								},
							},
						},
						FinishReason: "STOP",
						Index:        0,
					},
				},
				Usage: &llm.Usage{
					PromptTokens:     5,
					CompletionTokens: 2,
					TotalTokens:      7,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual := ToLLMSResponse(tc.input)

			// Deep-equality check as per project guidelines
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}

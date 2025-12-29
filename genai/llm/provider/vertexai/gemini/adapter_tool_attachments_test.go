package gemini

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestToRequest_ToolResultWithBinaryImageAttachment(t *testing.T) {
	type testCase struct {
		name     string
		input    *llm.GenerateRequest
		expected *Request
	}

	imageB64 := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})

	testCases := []testCase{
		{
			name: "tool message includes functionResponse and inline image",
			input: &llm.GenerateRequest{
				Messages: []llm.Message{
					{
						Role:       llm.RoleTool,
						Name:       "resources.readImage",
						ToolCallId: "call-1",
						Content:    `{"status":"ok"}`,
						Items: []llm.ContentItem{
							llm.NewBinaryContent([]byte{1, 2, 3}, "image/png", "img.png"),
							llm.NewTextContent(`{"status":"ok"}`),
						},
					},
				},
			},
			expected: &Request{
				Contents: []Content{
					{
						Role: "user",
						Parts: []Part{
							{
								FunctionResponse: &FunctionResponse{
									Name:     "resources.readImage",
									Response: map[string]interface{}{"status": "ok"},
								},
							},
							{
								InlineData: &InlineData{
									MimeType: "image/png",
									Data:     imageB64,
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToRequest(context.Background(), tc.input)
			assert.EqualValues(t, nil, err)
			assert.EqualValues(t, tc.expected, got)
		})
	}
}

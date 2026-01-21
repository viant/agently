package gemini

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestToRequest_ToolCallPlaceholderThoughtSignatureOmitted(t *testing.T) {
	req, err := ToRequest(context.Background(), &llm.GenerateRequest{
		Messages: []llm.Message{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						ID:        EMPTY_THOUGHT_SIGNATURE + "123",
						Name:      "test",
						Arguments: map[string]interface{}{"k": "v"},
					},
				},
			},
		},
	})
	if !assert.NoError(t, err) {
		return
	}

	var modelContent *Content
	for i := range req.Contents {
		if req.Contents[i].Role == "model" {
			modelContent = &req.Contents[i]
			break
		}
	}
	if !assert.NotNil(t, modelContent) {
		return
	}
	if !assert.Len(t, modelContent.Parts, 1) {
		return
	}
	assert.Equal(t, "", modelContent.Parts[0].ThoughtSignature)
}

func TestToRequest_ToolCallThoughtSignaturePreserved(t *testing.T) {
	req, err := ToRequest(context.Background(), &llm.GenerateRequest{
		Messages: []llm.Message{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						ID:        "  abc  ",
						Name:      "test",
						Arguments: map[string]interface{}{"k": "v"},
					},
				},
			},
		},
	})
	if !assert.NoError(t, err) {
		return
	}

	var modelContent *Content
	for i := range req.Contents {
		if req.Contents[i].Role == "model" {
			modelContent = &req.Contents[i]
			break
		}
	}
	if !assert.NotNil(t, modelContent) {
		return
	}
	if !assert.Len(t, modelContent.Parts, 1) {
		return
	}
	assert.Equal(t, "abc", modelContent.Parts[0].ThoughtSignature)
}

package stdio

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/agent/plan"
	mcpproto "github.com/viant/mcp-protocol/schema"
)

// TestPromptBasic ensures that user input collected by Prompt is returned and
// validated against a simple object schema.
func TestPromptBasic(t *testing.T) {
	jsonSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"favoriteDay": map[string]any{"type": "string"},
		},
		"required": []string{"favoriteDay"},
	}

	el := &plan.Elicitation{
		ElicitRequestParams: mcpproto.ElicitRequestParams{
			RequestedSchema: mcpproto.ElicitRequestParamsRequestedSchema{
				Type:       jsonSchema["type"].(string),
				Properties: jsonSchema["properties"].(map[string]any),
				Required:   jsonSchema["required"].([]string),
			},
		},
	}

	// Simulate user entering "Friday" followed by newline.
	in := bytes.NewBufferString("Friday\n")
	var out bytes.Buffer

	res, err := Prompt(context.Background(), &out, in, el)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	expected := map[string]any{"favoriteDay": "Friday"}

	assert.EqualValues(t, plan.ElicitResultActionAccept, res.Action)
	assert.EqualValues(t, expected, res.Payload)
}

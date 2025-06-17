package stdio

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/agent/plan"
)

// TestPromptBasic ensures that user input collected by Prompt is returned and
// validated against a simple object schema.
func TestPromptBasic(t *testing.T) {
	jsonSchema := `{"type":"object","properties":{"favoriteDay":{"type":"string"}},"required":["favoriteDay"]}`
	el := &plan.Elicitation{Schema: jsonSchema}

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

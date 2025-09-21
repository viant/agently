package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/viant/agently/genai/agent/plan"
	core2 "github.com/viant/agently/genai/service/core"
	executil "github.com/viant/agently/genai/service/shared/executil"
)

// dd test for extendPlanFromContent: parses elicitation JSON embedded in content.
func TestService_extendPlanFromContent_DD(t *testing.T) {
	ctx := context.Background()
	s := &Service{}

	type testCase struct {
		name     string
		content  string
		expected *plan.Elicitation
	}

	// Provided elicitation JSON
	elicitationJSON := `{
"type": "elicitation",
"message": "To find out how many tables are in your ci_ads database, I need the connection details for that database so I can access it.\nPlease provide the following information:",
"requestedSchema": {
"type": "object",
"properties": {
"name": { "type": "string", "description": "Connector name you’d like to assign (e.g., ci_ads_conn)" },
"driver": { "type": "string", "enum": ["postgres", "mysql", "bigquery"], "description": "Database type/driver" },
"host": { "type": "string", "description": "Hostname or IP of the database server" },
"port": { "type": integer, "description": "Port number the database listens on" },
"db": { "type": "string", "description": "Database name (ci_ads)" }
},
"required": ["name", "driver", "host", "port", "db"]
}
}`

	// Build expected by unmarshalling the same content into plan.Elicitation generically.
	expected := &plan.Elicitation{}
	_ = executil.EnsureJSONResponse(ctx, elicitationJSON, expected)
	if expected.IsEmpty() {
		expected = nil
	}

	cases := []testCase{
		{
			name:     "elicitation from content",
			content:  elicitationJSON,
			expected: expected,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &core2.GenerateOutput{Content: tc.content}
			aPlan := plan.New()
			err := s.extendPlanFromContent(ctx, out, aPlan)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.expected, aPlan.Elicitation)
		})
	}
}

package resolver

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func mustUnmarshal(t *testing.T, s string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return v
}

func TestSelect(t *testing.T) {
	input := mustUnmarshal(t, `{"args": {"name": "john", "items": [1,2,3]}}`)
	output := mustUnmarshal(t, `{"plan": [{"step":"scan","status":"completed"},{"step":"draft","status":"in_progress"}], "explanation": "ok"}`)

	cases := []struct {
		name string
		sel  string
		want interface{}
	}{
		{"root_output", "output", output},
		{"root_input", "input", input},
		{"default_to_output", "plan", mustUnmarshal(t, `[{"step":"scan","status":"completed"},{"step":"draft","status":"in_progress"}]`)},
		{"output_field", "output.explanation", "ok"},
		{"array_index_dot", "output.plan.0.step", "scan"},
		{"array_index_bracket", "output.plan[1].status", "in_progress"},
		{"input_nested", "input.args.name", "john"},
		{"input_array", "input.args.items.2", float64(3)},
		{"missing_path", "output.missing.key", nil},
		{"bad_index", "output.plan[x]", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Select(tc.sel, input, output)
			assert.EqualValues(t, tc.want, got)
		})
	}
}

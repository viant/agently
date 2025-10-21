package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestService_List_DataDriven(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name     string
		items    []ListItem
		expected *ListOutput
	}{
		{
			name:     "empty list",
			items:    nil,
			expected: &ListOutput{Items: nil},
		},
		{
			name:     "single item",
			items:    []ListItem{{ID: "coder", Name: "Coder", Description: "Writes code", Priority: 10, Tags: []string{"code"}}},
			expected: &ListOutput{Items: []ListItem{{ID: "coder", Name: "Coder", Description: "Writes code", Priority: 10, Tags: []string{"code"}}}},
		},
		{
			name: "multiple items",
			items: []ListItem{
				{ID: "researcher", Name: "Researcher", Description: "Finds info", Priority: 5, Tags: []string{"research"}},
				{ID: "coder", Name: "Coder", Description: "Writes code", Priority: 10, Tags: []string{"code"}},
			},
			expected: &ListOutput{Items: []ListItem{
				{ID: "researcher", Name: "Researcher", Description: "Finds info", Priority: 5, Tags: []string{"research"}},
				{ID: "coder", Name: "Coder", Description: "Writes code", Priority: 10, Tags: []string{"code"}},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			dir := func() []ListItem { return tc.items }
			s := New(nil, WithDirectoryProvider(dir))

			// Act
			var out ListOutput
			err := s.list(ctx, &struct{}{}, &out)

			// Assert
			assert.NoError(t, err)
			assert.EqualValues(t, tc.expected, &out)
		})
	}
}

func TestService_Run_External_DataDriven(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name     string
		input    *RunInput
		runner   func(context.Context, string, string, map[string]interface{}) (string, string, string, string, bool, []string, error)
		expected *RunOutput
	}{
		{
			name:  "external returns task and context",
			input: &RunInput{AgentID: "researcher", Objective: "find info", Context: map[string]interface{}{"foo": "bar"}},
			runner: func(_ context.Context, agentID, objective string, payload map[string]interface{}) (string, string, string, string, bool, []string, error) {
				return "answer", "completed", "t-1", "ctx-1", true, []string{"warn-1"}, nil
			},
			expected: &RunOutput{Answer: "answer", Status: "completed", TaskID: "t-1", ContextID: "ctx-1", StreamSupported: true, Warnings: []string{"warn-1"}},
		},
		{
			name:  "external returns empty answer but terminal status",
			input: &RunInput{AgentID: "ext", Objective: "do"},
			runner: func(_ context.Context, agentID, objective string, payload map[string]interface{}) (string, string, string, string, bool, []string, error) {
				return "", "failed", "t-err", "ctx-x", false, nil, nil
			},
			expected: &RunOutput{Answer: "", Status: "failed", TaskID: "t-err", ContextID: "ctx-x", StreamSupported: false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			s := New(nil, WithExternalRunner(tc.runner))

			// Act
			var out RunOutput
			err := s.run(ctx, tc.input, &out)

			// Assert
			assert.NoError(t, err)
			assert.EqualValues(t, tc.expected, &out)
		})
	}
}

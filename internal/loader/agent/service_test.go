package agent

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	_ "github.com/viant/afs/embed"
	"github.com/viant/agently/genai/agent"
	"github.com/viant/fluxor/service/meta"
	"testing"
)

// testFS holds our test YAML files
//
//go:embed testdata/*
var testFS embed.FS

// TestService_Load tests the agent loading functionality
func TestService_Load(t *testing.T) {
	// Set up memory file system
	ctx := context.Background()

	// Test cases
	testCases := []struct {
		name         string
		url          string
		expectedJSON string
		expectedErr  bool
	}{
		{
			name:         "Valid agent",
			url:          "tester.yaml",
			expectedJSON: `{"id":"agent-123","name":"Database tester Agent","icon":"https://example.com/icon.png","source":{"url":"tester.yaml"},"model":"o1","temperature":0.7,"description":"An example agent for demonstration purposes.","knowledge":[{"match":{"Inclusions":["*.md"]},"url":"knowledge/"}]}`,
		},
		{
			name:        "Invalid URL",
			url:         "nonexistent.yaml",
			expectedErr: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := New(WithMetaService(meta.New(afs.New(), "embed:///testdata", &testFS)))
			anAgent, err := service.Load(ctx, tc.url)

			if tc.expectedErr {
				assert.NotNil(t, err)
				return
			}
			expected := &agent.Agent{}
			err = json.Unmarshal([]byte(tc.expectedJSON), expected)
			if !assert.EqualValues(t, expected, anAgent) {
				actualJSON, err := json.Marshal(anAgent)
				fmt.Println(string(actualJSON), err)
			}
		})
	}
}

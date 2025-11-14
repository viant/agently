package agent

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	_ "github.com/viant/afs/embed"
	"github.com/viant/agently/genai/agent"
	meta "github.com/viant/agently/internal/workspace/service/meta"
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
			expectedJSON: `{"id":"agent-123","name":"Database tester Agent","icon":"https://example.com/icon.png","source":{"url":"embed:///testdata/tester.yaml"},"model":"o1","temperature":0.7,"description":"An example agent for demonstration purposes.","knowledge":[{"match":{"Inclusions":["*.md"]},"url":"embed://localhost/testdata/knowledge"}]}`,
		},
		{
			name: "Agent with chains",
			url:  "with_chains.yaml",
			expectedJSON: `{
			  "id":"agent-chain-demo",
			  "name":"chainLimits Demo",
			  "source":{"url":"embed:///testdata/with_chains.yaml"},
			  "model":"gpt-4o",
            "chains":[
                {"on":"succeeded","target":{"agentId":"summarizer"},"mode":"sync","conversation":"link","query":{"text":"Summarize the assistant reply: {{ .Output.Content }}"},"publish":{"role":"assistant"}},
			    {"on":"failed","target":{"agentId":"notifier"},"mode":"sync","conversation":"reuse","when":"{{ ne .Output.Content \"\" }}","onError":"message"}
			  ]
			}`,
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
			service := New(WithMetaService(meta.New(afs.New(), "embed:///testdata")))
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

func TestService_Load_UIFlags(t *testing.T) {
	ctx := context.Background()
	service := New(WithMetaService(meta.New(afs.New(), "embed:///testdata")))

	got, err := service.Load(ctx, "flags.yaml")
	assert.NoError(t, err)

	// All three flags are provided as false in YAML and must be parsed as such
	if assert.NotNil(t, got.ShowExecutionDetails, "ShowExecutionDetails must be set") {
		assert.False(t, *got.ShowExecutionDetails, "ShowExecutionDetails should be false")
	}
	if assert.NotNil(t, got.ShowToolFeed, "ShowToolFeed must be set") {
		assert.False(t, *got.ShowToolFeed, "ShowToolFeed should be false")
	}
	if assert.NotNil(t, got.AutoSummarize, "AutoSummarize must be set") {
		assert.False(t, *got.AutoSummarize, "AutoSummarize should be false")
	}
}

func TestService_Load_ToolExposure(t *testing.T) {
	ctx := context.Background()
	service := New(WithMetaService(meta.New(afs.New(), "embed:///testdata")))

	// Minimal, focused assertions: exposure must be set consistently
	t.Run("tool.callExposure alias is parsed", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_callExposure.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tools) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tools.CallExposure)
		}
	})

	t.Run("new tool block with toolCallExposure", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_new.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tools) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tools.CallExposure)
		}
	})

	t.Run("tool.callexposure (lowercase) is parsed", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_callexposure.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tools) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tools.CallExposure)
		}
	})

	t.Run("top-level toolCallExposure mirrors into tool block", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_top.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tools) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tools.CallExposure)
		}
	})
}

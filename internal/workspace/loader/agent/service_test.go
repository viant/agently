package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viant/afs"
	"github.com/viant/agently/genai/agent"
	meta "github.com/viant/agently/internal/workspace/service/meta"
	yml "github.com/viant/agently/internal/workspace/service/meta/yml"
	"gopkg.in/yaml.v3"
)

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
			name: "Valid agent",
			url:  "tester.yaml",
			expectedJSON: `{
  "id":"agent-123",
  "name":"Database tester Agent",
  "icon":"https://example.com/icon.png",
  "source":{"url":"testdata/tester.yaml"},
  "model":"o1",
  "temperature":0.7,
  "description":"An example agent for demonstration purposes.",
  "knowledge":[{"filter":{"Exclusions":null,"Inclusions":["*.md"],"MaxFileSize":0},"url":"knowledge/"}],
  "resources":[{"uri":"knowledge/","role":"user","allowSemanticMatch":true}],
  "tool":{}
}`,
		},
		{
			name: "Agent with chains",
			url:  "with_chains.yaml",
			expectedJSON: `{
			  "id":"agent-chain-demo",
			  "name":"Chain Demo",
			  "source":{"url":"testdata/with_chains.yaml"},
			  "model":"gpt-4o",
            "chains":[
                {"on":"succeeded","target":{"agentId":"summarizer"},"mode":"sync","conversation":"link","query":{"text":"Summarize the assistant reply: {{ .Output.Content }}"},"publish":{"role":"assistant"}},
			    {"on":"failed","target":{"agentId":"notifier"},"mode":"sync","conversation":"reuse","when":{"expr":"{{ ne .Output.Content \"\" }}"},"onError":"message"}
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
			service := New(WithMetaService(meta.New(afs.New(), "testdata")))
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
	service := New(WithMetaService(meta.New(afs.New(), "testdata")))

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
	service := New(WithMetaService(meta.New(afs.New(), "testdata")))

	// Minimal, focused assertions: exposure must be set consistently
	t.Run("tool.callExposure alias is parsed", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_callExposure.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tool) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tool.CallExposure)
		}
	})

	t.Run("new tool block with toolCallExposure", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_new.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tool) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tool.CallExposure)
		}
	})

	t.Run("tool.callexposure (lowercase) is parsed", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_callexposure.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tool) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tool.CallExposure)
		}
	})

	t.Run("top-level toolCallExposure mirrors into tool block", func(t *testing.T) {
		got, err := service.Load(ctx, "tool_top.yaml")
		assert.NoError(t, err)
		if assert.NotNil(t, got) && assert.NotNil(t, got.Tool) {
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.ToolCallExposure)
			assert.EqualValues(t, agent.ToolCallExposure("conversation"), got.Tool.CallExposure)
		}
	})
}

func TestService_Load_ToolBundles(t *testing.T) {
	ctx := context.Background()
	service := New(WithMetaService(meta.New(afs.New(), "testdata")))

	testCases := []struct {
		name            string
		url             string
		expectedBundles []string
		expectedExpo    agent.ToolCallExposure
	}{
		{
			name:            "tool.bundles are parsed from mapping tool block",
			url:             "tool_bundles.yaml",
			expectedBundles: []string{"system/exec", "system/os"},
			expectedExpo:    agent.ToolCallExposure("conversation"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := service.Load(ctx, testCase.url)
			require.NoError(t, err)
			require.NotNil(t, got)

			actualBundles := []string(nil)
			if got.Tool.Bundles != nil {
				actualBundles = append([]string(nil), got.Tool.Bundles...)
			}
			assert.EqualValues(t, testCase.expectedBundles, actualBundles)
			assert.EqualValues(t, testCase.expectedExpo, got.Tool.CallExposure)
			assert.EqualValues(t, testCase.expectedExpo, got.ToolCallExposure)
		})
	}
}

func TestParseResourceEntry_SystemFlag(t *testing.T) {
	makeNode := func(doc string) *yml.Node {
		var root yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(doc), &root))
		require.Greater(t, len(root.Content), 0)
		return (*yml.Node)(root.Content[0])
	}

	t.Run("system true sets role", func(t *testing.T) {
		node := makeNode(`uri: workspace://foo
system: true`)
		res, err := parseResourceEntry(node)
		assert.NoError(t, err)
		if assert.NotNil(t, res) {
			assert.Equal(t, "system", res.Role)
		}
	})

	t.Run("system false keeps user role", func(t *testing.T) {
		node := makeNode(`uri: workspace://bar
role: user
system: false`)
		res, err := parseResourceEntry(node)
		assert.NoError(t, err)
		if assert.NotNil(t, res) {
			assert.Equal(t, "user", res.Role)
		}
	})

	t.Run("conflicting role and system raises error", func(t *testing.T) {
		node := makeNode(`uri: workspace://baz
role: user
system: true`)
		_, err := parseResourceEntry(node)
		assert.Error(t, err)
	})
}

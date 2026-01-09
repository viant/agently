package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	agentsvc "github.com/viant/agently/genai/service/agent"
	toolpol "github.com/viant/agently/genai/tool"
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

// fakeAgentRuntime is a lightweight stub implementing agentRuntime so we can
// verify that llm/agents:run threads model preferences and reasoning effort
// through to the underlying agent.Query input.
type fakeAgentRuntime struct {
	lastInput  *agentsvc.QueryInput
	lastPolicy *toolpol.Policy
	finder     agentmdl.Finder
}

func (f *fakeAgentRuntime) Query(ctx context.Context, in *agentsvc.QueryInput, out *agentsvc.QueryOutput) error {
	f.lastInput = in
	f.lastPolicy = toolpol.FromContext(ctx)
	if out != nil {
		out.Content = "ok"
	}
	return nil
}

// Finder is unused in this test; return nil to satisfy the interface.
func (f *fakeAgentRuntime) Finder() agentmdl.Finder { return f.finder }

func TestService_Run_Internal_ThreadsModelPrefsAndReasoning(t *testing.T) {
	ctx := context.Background()
	streaming := false
	reasoning := "medium"
	prefs := &llm.ModelPreferences{
		IntelligencePriority: 0.7,
		SpeedPriority:        0.7,
		CostPriority:         0.7,
	}
	in := &RunInput{
		AgentID:          "dev_reviewer",
		Objective:        "review repo",
		Streaming:        &streaming,
		ModelPreferences: prefs,
		ReasoningEffort:  &reasoning,
		Context:          map[string]interface{}{"foo": "bar"},
	}

	fake := &fakeAgentRuntime{}
	s := &Service{agent: fake}
	var out RunOutput
	err := s.run(ctx, in, &out)
	assert.NoError(t, err)
	if assert.NotNil(t, fake.lastInput, "expected QueryInput to be passed to agent runtime") {
		assert.Equal(t, in.AgentID, fake.lastInput.AgentID)
		assert.Equal(t, in.Objective, fake.lastInput.Query)
		assert.Equal(t, in.Context, fake.lastInput.Context)
		assert.Equal(t, prefs, fake.lastInput.ModelPreferences)
		assert.Equal(t, &reasoning, fake.lastInput.ReasoningEffort)
	}
}

func TestService_Run_Internal_DoesNotInheritParentToolAllowList(t *testing.T) {
	streaming := false
	in := &RunInput{
		AgentID:   "dev_reviewer",
		Objective: "review repo",
		Streaming: &streaming,
		Context:   map[string]interface{}{"foo": "bar"},
	}

	testCases := []struct {
		name           string
		ctx            context.Context
		expectedPolicy *toolpol.Policy
	}{
		{
			name:           "no_parent_policy",
			ctx:            context.Background(),
			expectedPolicy: nil,
		},
		{
			name: "parent_policy_is_cleared",
			ctx: toolpol.WithPolicy(context.Background(), &toolpol.Policy{
				Mode:      toolpol.ModeAuto,
				AllowList: []string{"llm/agents:run"},
			}),
			expectedPolicy: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAgentRuntime{}
			s := &Service{agent: fake}

			var out RunOutput
			err := s.run(tc.ctx, in, &out)

			assert.NoError(t, err)
			if assert.NotNil(t, fake.lastInput) {
				assert.EqualValues(t, []string{}, fake.lastInput.ToolsAllowed)
			}
			assert.EqualValues(t, tc.expectedPolicy, fake.lastPolicy)
		})
	}
}

type fakeFinder struct {
	agents map[string]*agentmdl.Agent
}

func (f *fakeFinder) Find(_ context.Context, id string) (*agentmdl.Agent, error) {
	if f == nil || f.agents == nil {
		return nil, nil
	}
	return f.agents[id], nil
}

func TestService_Run_PrefersInternalWhenResolvable(t *testing.T) {
	ctx := context.Background()
	calledExternal := false

	testCases := []struct {
		name           string
		intendedSource string
		expectExternal bool
	}{
		{name: "unknown_route_prefers_internal", intendedSource: "", expectExternal: false},
		{name: "explicit_external_route_uses_external", intendedSource: "external", expectExternal: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			calledExternal = false
			runner := func(_ context.Context, agentID, objective string, payload map[string]interface{}) (string, string, string, string, bool, []string, error) {
				calledExternal = true
				return "ext-answer", "completed", "t-1", "ctx-1", true, nil, nil
			}
			in := &RunInput{AgentID: "coder", Objective: "do work"}
			fake := &fakeAgentRuntime{
				finder: &fakeFinder{agents: map[string]*agentmdl.Agent{"coder": {Identity: agentmdl.Identity{ID: "coder"}}}},
			}
			s := &Service{
				agent:       fake,
				runExternal: runner,
				allowed:     map[string]string{"coder": tc.intendedSource},
			}

			var out RunOutput
			err := s.run(ctx, in, &out)

			assert.NoError(t, err)
			assert.EqualValues(t, tc.expectExternal, calledExternal)
			if tc.expectExternal {
				assert.EqualValues(t, "ext-answer", out.Answer)
			} else {
				assert.EqualValues(t, "ok", out.Answer)
			}
		})
	}
}

func TestService_Run_Strict_AllowsInternalNotListedInDirectory(t *testing.T) {
	ctx := context.Background()
	in := &RunInput{AgentID: "internal-only", Objective: "do work"}

	testCases := []struct {
		name    string
		allowed map[string]string
		items   []ListItem
		wantErr bool
	}{
		{
			name:    "internal_allowed_but_not_listed",
			allowed: map[string]string{"internal-only": "internal"},
			items:   []ListItem{{ID: "public", Name: "Public", Source: "internal"}},
			wantErr: false,
		},
		{
			name:    "not_allowed_fails_in_strict",
			allowed: map[string]string{},
			items:   []ListItem{{ID: "public", Name: "Public", Source: "internal"}},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAgentRuntime{}
			s := New(nil,
				WithDirectoryProvider(func() []ListItem { return tc.items }),
				WithAllowedIDs(tc.allowed),
				WithStrict(true),
			)
			s.agent = fake

			var out RunOutput
			err := s.run(ctx, in, &out)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, "ok", out.Answer)
		})
	}
}

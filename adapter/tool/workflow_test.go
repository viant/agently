package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/model"
)

// TestRegisterWorkflowAsTool verifies that a workflow is exposed as a tool
// definition with expected naming convention.
func TestRegisterWorkflowAsTool(t *testing.T) {
	testCases := []struct {
		name   string
		wfName string
	}{
		{name: "simple", wfName: "my_workflow"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			rt := &fluxor.Runtime{}
			registry := tool.NewRegistry()
			wf := model.NewWorkflow(tc.wfName)

			// when
			RegisterWorkflow(rt, wf, registry)

			// then
			def, ok := registry.GetDefinition("wf_" + tc.wfName)
			assert.True(t, ok, "tool definition should be registered")
			assert.EqualValues(t, "wf_"+tc.wfName, def.Name)
		})
	}
}

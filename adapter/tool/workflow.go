// Package tooladapter groups helpers that expose various runtime components
// (Fluxor workflow, MCP server, etc.) as first-class LLM tools and/or Fluxor
// services.
//
// This file implements the Workflow → LLM tool adapter that previously lived
// in internal/adapter/fluxorllm.

package tool

import (
	"context"
	"encoding/json"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor"
	"github.com/viant/fluxor/model"
	"time"
)

// RegisterWorkflow registers a Fluxor workflow as an LLM tool (function).
//
// The tool name is "wf_<workflow.Name>".  When the LLM invokes the tool the
// workflow is started in the supplied runtime with the arguments map forwarded
// as input payload.  The handler waits (max 1h) for the workflow to finish and
// returns the final output JSON-encoded.
//
// The helper is idempotent – if the registry already contains a definition
// with the chosen name, the call is a no-op.  Nil arguments are ignored.
func RegisterWorkflow(rt *fluxor.Runtime, wf *model.Workflow, registry *tool.Registry) {
	if rt == nil || wf == nil || registry == nil {
		return
	}

	name := "wf_" + wf.Name
	if _, exists := registry.GetDefinition(name); exists {
		return // already registered
	}

	def := llm.ToolDefinition{
		Name:        name,
		Description: wf.Description,
		Parameters: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		},
	}

	handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		if args == nil {
			args = map[string]interface{}{}
		}

		_, wait, err := rt.StartProcess(ctx, wf, args)
		if err != nil {
			return "", err
		}

		result, err := wait(ctx, time.Hour)
		if err != nil {
			return "", err
		}

		data, _ := json.Marshal(result.Output)
		return string(data), nil
	}

	registry.Register(def, handler)
}

// RegisterWorkflowAsTool is kept for backward compatibility with earlier
// versions that exposed the helper under this exact name inside a different
// package path.  New code should call RegisterWorkflow.
func RegisterWorkflowAsTool(rt *fluxor.Runtime, wf *model.Workflow, registry *tool.Registry) {
    RegisterWorkflow(rt, wf, registry)
}

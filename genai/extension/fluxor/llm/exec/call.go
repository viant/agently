package exec

import (
	"context"
	_ "embed"
	"fmt"

	runner "github.com/viant/agently/genai/extension/fluxor/llm/shared/executil"
	"github.com/viant/agently/genai/tool"
)

type CallToolInput struct {
	Name           string                 `yaml:"name,omitempty" json:"name,omitempty"`   // Specific tool/function name (if applicable)
	Args           map[string]interface{} `yaml:"args,omitempty" json:"args,omitempty"`   // Tool arguments (must match the schema in tool_definitions)
	Model          string                 `yaml:"model,omitempty" json:"model,omitempty"` // LLM model to use for planning
	Retries        int                    `yaml:"retries,omitempty" json:"retries,omitempty"`
	PromptTemplate string                 `yaml:"promptTemplate" json:"promptTemplate,omitempty"` // optional custom prompt
	Results        string                 `yaml:"context,omitempty" json:"context,omitempty"`
}

type CallToolOutput struct {
	Result string `yaml:"result,omitempty" json:"result,omitempty"` // Result content from tool execution
}

// executePlan iterates over plan steps, calling tools for 'tool' steps.
func (s *Service) callTool(ctx context.Context, in, out interface{}) error {
	input := in.(*CallToolInput)
	output := out.(*CallToolOutput)
	return s.CallTool(ctx, input, output)
}

func (s *Service) CallTool(ctx context.Context, input *CallToolInput, output *CallToolOutput) error {
	// ------------------------------------------------------------
	// Execute tool via injected registry or default
	// ------------------------------------------------------------
	if pol := tool.FromContext(ctx); pol != nil {
		if !pol.IsAllowed(input.Name) {
			return fmt.Errorf("tool %s is not allowed by policy", input.Name)
		}
		if pol.Mode == tool.ModeDeny {
			return fmt.Errorf("tool %s execution denied by policy", input.Name)
		}
	}

	// Execute via shared runner (no trace by default in CallTool)
	step := runner.StepInfo{ID: "", Name: input.Name, Args: input.Args}
	res, _, _, err := runner.RunTool(ctx, s.registry, step, runner.WithRecorder(s.recorder))
	output.Result = res.Result
	return err
}

func statusFromErr(err error) string {
	if err != nil {
		return "failed"
	}
	return "completed"
}

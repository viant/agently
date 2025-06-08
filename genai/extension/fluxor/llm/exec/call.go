package exec

import (
	"context"
	_ "embed"
	"fmt"

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
	// Execute tool via injected registry or default
	var executor func(ctx context.Context, name string, args map[string]interface{}) (string, error)
	executor = s.registry.Execute

	// ------------------------------------------------------------------
	// Back-compat / sensible defaults for common tools
	// ------------------------------------------------------------------
	// Older LLM prompts – and some user-generated follow-up plans – often
	// omit optional parameters that the JSON schema marks as *required*.
	// Rather than failing the entire workflow for a missing timeout or
	// abort flag, fall back to conservative defaults so that the tool
	// still executes and the user receives a meaningful answer.

	// --- policy evaluation -------------------------------------------------
	// Tool approval is handled uniformly by the outer workflow via the Fluxor
	// executor.  Requesting an additional approval here would duplicate the
	// prompt.  We therefore enforce deny/allow lists but skip the explicit
	// Ask branch – the decision will be captured once when the actual tool
	// action (e.g. system/exec.execute) runs.

	if pol := tool.FromContext(ctx); pol != nil {
		if !pol.IsAllowed(input.Name) {
			return fmt.Errorf("tool %s is not allowed by policy", input.Name)
		}
		if pol.Mode == tool.ModeDeny {
			return fmt.Errorf("tool %s execution denied by policy", input.Name)
		}
	}

	toolResult, err := executor(ctx, input.Name, input.Args)
	output.Result = toolResult
	return err
}

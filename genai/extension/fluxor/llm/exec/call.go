package exec

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/tool"
	"time"
)

//go:embed prompt/refine_tool_call.vm
var toolCallRefineTemplate string

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

	// --- policy evaluation -------------------------------------------------
	if pol := tool.FromContext(ctx); pol != nil {
		if !pol.IsAllowed(input.Name) {
			return fmt.Errorf("tool %s is not allowed by policy", input.Name)
		}
		switch pol.Mode {
		case tool.ModeDeny:
			return fmt.Errorf("tool %s execution denied by policy", input.Name)
		case tool.ModeAsk:
			if pol.Ask == nil {
				return errors.New("policy mode=ask but Ask callback is nil")
			}
			approved := pol.Ask(ctx, input.Name, input.Args, pol)
			if !approved {
				return fmt.Errorf("tool %s execution rejected by user", input.Name)
			}
			// pol may have been modified (e.g. switched to auto)
		}
	}

	// Determine retry count: use step-specific retries if set, else default to s.MaxRetries
	retries := max(3, input.Retries)

	var toolResult string
	var err error
	// Retry loop
	for attempt := 1; attempt <= retries; attempt++ {
		if attempt-1 == retries {
			return fmt.Errorf("tool %s execution error after %d attempts: %w", input.Name, retries, err)
		}
		planningModel := input.Model
		if planningModel == "" {
			planningModel = s.defaultModel
		}
		toolResult, err = executor(ctx, input.Name, input.Args)
		if err != nil {
			// Attempt parameter adjustment via LLM if available
			if s.llm != nil && planningModel != "" {

				promptTemplate := input.PromptTemplate
				if promptTemplate == "" {
					promptTemplate = toolCallRefineTemplate
				}
				refinedOutput := core.GenerateOutput{}
				if gErr := s.llm.Generate(ctx, &core.GenerateInput{
					Model:    planningModel,
					Template: promptTemplate,
					Bind: map[string]interface{}{
						"Tool":  input.Name,
						"Args":  input.Args,
						"Error": err.Error(),
					},
				}, &refinedOutput); gErr == nil {
					var args map[string]interface{}
					if jErr := core.EnsureJSONResponse(ctx, refinedOutput.Content, &args); jErr == nil {
						input.Args = args
						continue
					}
				}
				// Simple exponential back-off: 100ms, 200ms, 400ms …
				time.Sleep(time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond)
				continue
			}
		}
		if toolResult == "" && attempt < retries {
			continue
		}
		break
	}
	output.Result = toolResult
	return err

}

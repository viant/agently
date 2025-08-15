package exec

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/viant/agently/genai/tool"
	elog "github.com/viant/agently/internal/log"
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
	var executor func(ctx context.Context, name string, args map[string]interface{}) (string, error)
	executor = s.registry.Execute
	if pol := tool.FromContext(ctx); pol != nil {
		if !pol.IsAllowed(input.Name) {
			return fmt.Errorf("tool %s is not allowed by policy", input.Name)
		}
		if pol.Mode == tool.ModeDeny {
			return fmt.Errorf("tool %s execution denied by policy", input.Name)
		}
	}

	// Publish TaskInput event before execution so that sinks can capture the
	// tool invocation with its arguments.
	elog.Publish(elog.Event{
		Time:      time.Now(),
		EventType: elog.TaskInput,
		Payload: map[string]interface{}{
			"tool": input.Name,
			"args": input.Args,
		},
	})

	toolResult, err := executor(ctx, input.Name, input.Args)
	output.Result = toolResult

	// Log the output/result (and potential error) so that it is captured by the
	// default FileSink (typically writing to agently.log).
	elog.Publish(elog.Event{
		Time:      time.Now(),
		EventType: elog.TaskOutput,
		Payload: map[string]interface{}{
			"tool":   input.Name,
			"result": toolResult,
			"error":  err,
		},
	})
	return err
}

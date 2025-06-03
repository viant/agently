package exec

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/viant/agently/genai/tool"
	approval "github.com/viant/fluxor/service/approval"
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
	if pol := tool.FromContext(ctx); pol != nil {
		if !pol.IsAllowed(input.Name) {
			return fmt.Errorf("tool %s is not allowed by policy", input.Name)
		}
		switch pol.Mode {
		case tool.ModeDeny:
			return fmt.Errorf("tool %s execution denied by policy", input.Name)
		case tool.ModeAsk:
			// delegate to Fluxor approval service for interactive confirmation
			if s.approvalService == nil {
				return fmt.Errorf("policy mode=ask but ApprovalService is not available")
			}
			raw, err := json.Marshal(input.Args)
			if err != nil {
				return fmt.Errorf("failed to marshal args for approval: %w", err)
			}
			req := &approval.Request{Action: input.Name, Args: raw, ID: uuid.New().String()}
			if err := s.approvalService.RequestApproval(ctx, req); err != nil {
				return fmt.Errorf("failed to request approval for tool %s: %w", input.Name, err)
			}
			dec, err := approval.WaitForDecision(ctx, s.approvalService, req.ID, 0)
			if err != nil {
				return fmt.Errorf("failed to await approval decision for tool %s: %w", input.Name, err)
			}
			if !dec.Approved {
				return fmt.Errorf("tool %s execution rejected by user: %s", input.Name, dec.Reason)
			}
		}
	}

	toolResult, err := executor(ctx, input.Name, input.Args)
	output.Result = toolResult
	return err
}

package service

import (
	"context"
	"time"

	agentpkg "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/tool"
)

// RunRequest represents a one-shot workflow execution request using the
// extended QueryInput structure (location, documents, limits, …).
type RunRequest struct {
	Input *agentpkg.QueryInput // required

	Policy  *tool.Policy
	Timeout time.Duration
}

// Run executes the provided workflow and returns the raw QueryOutput.  No
// elicitation loop is attempted – caller is responsible for checking
// output.Elicitation.
func (s *Service) Run(ctx context.Context, req RunRequest) (*agentpkg.QueryOutput, error) {
	if s == nil || s.exec == nil {
		return nil, nil
	}

	if req.Input == nil {
		return nil, nil
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	if req.Policy != nil {
		ctx = tool.WithPolicy(ctx, req.Policy)
	}

	out, err := s.exec.Conversation().Accept(ctx, req.Input)
	return out, err
}

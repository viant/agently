package service

import (
	"context"
	"fmt"
	"time"

	"github.com/viant/agently/genai/usage"
)

// WorkflowRequest is retained for backward compatibility. In decoupled mode,
// workflow execution is not available and requests will return an error.
type WorkflowRequest struct {
	// Location points to a YAML/JSON workflow definition. It can be an
	// absolute URL, a filesystem path or a relative path resolved by the
	// executor's meta service.
	Location string

	// TaskID optionally limits execution to a single task inside the
	// workflow (via Runtime.RunTaskOnce). Leave empty to run the whole
	// process from the workflow's root node(s).
	TaskID string

	// Input passed as the execution input or initial state.
	Input interface{}

	// Timeout caps the total execution time. Zero means no timeout.
	Timeout time.Duration
}

// WorkflowResponse captures the execution result.
type WorkflowResponse struct {
	Output interface{}
	Usage  *usage.Aggregator
}

// ExecuteWorkflow loads the workflow located at req.Location and executes it
// using the shared runtime. When TaskID is empty the entire workflow is run;
// otherwise only the specified task is executed. The method waits until
// completion or timeout.
func (s *Service) ExecuteWorkflow(ctx context.Context, req WorkflowRequest) (*WorkflowResponse, error) {
	return nil, fmt.Errorf("workflow orchestration is not available in decoupled mode")
}

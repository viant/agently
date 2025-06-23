package service

import (
	"context"
	"fmt"
	"time"

	"github.com/viant/agently/genai/usage"
)

// WorkflowRequest describes parameters to execute a Fluxor workflow.
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

// ExecuteWorkflow loads the workflow located at req.AgentName and executes it
// using the shared runtime. When TaskID is empty the entire workflow is run;
// otherwise only the specified task is executed. The method waits until
// completion or timeout.
func (s *Service) ExecuteWorkflow(ctx context.Context, req WorkflowRequest) (*WorkflowResponse, error) {
	if s == nil || s.exec == nil {
		return nil, fmt.Errorf("service not initialised")
	}

	orch := s.exec.Orchestration()
	if orch == nil {
		return nil, fmt.Errorf("orchestration runtime not initialised")
	}

	runtime := orch.WorkflowRuntime()
	if runtime == nil {
		return nil, fmt.Errorf("workflow runtime is nil")
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	ctx, agg := usage.WithAggregator(ctx)

	wf, err := runtime.LoadWorkflow(ctx, req.Location)
	if err != nil {
		return nil, fmt.Errorf("load workflow: %w", err)
	}

	var output interface{}

	if req.TaskID != "" {
		// Single task execution path.
		output, err = runtime.RunTaskOnce(ctx, wf, req.TaskID, req.Input)
		if err != nil {
			return nil, err
		}
	} else {
		// Full workflow execution path; start a process and wait.
		initial := map[string]interface{}{}
		if req.Input != nil {
			initial["input"] = req.Input
		}

		_, waitFn, err := runtime.StartProcess(ctx, wf, initial)
		if err != nil {
			return nil, err
		}

		// Use provided timeout or default wait.
		waitTimeout := req.Timeout
		if waitTimeout == 0 {
			waitTimeout = 30 * time.Minute
		}

		procOut, err := waitFn(ctx, waitTimeout)
		if err != nil {
			return nil, err
		}
		if procOut != nil {
			output = procOut.Output
		}
	}

	return &WorkflowResponse{Output: output, Usage: agg}, nil
}

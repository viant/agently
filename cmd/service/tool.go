package service

import (
	"context"
	"time"
)

// ToolRequest parameters for executing a single tool.
type ToolRequest struct {
	Name    string                 // fully qualified tool name
	Args    map[string]interface{} // JSON-compatible arguments
	Timeout time.Duration          // optional per call timeout
}

type ToolResponse struct {
	Result interface{}
}

// ExecuteTool delegates to the underlying executor's tool registry.
func (s *Service) ExecuteTool(ctx context.Context, req ToolRequest) (*ToolResponse, error) {
	if s == nil || s.exec == nil {
		return nil, nil
	}
	out, err := s.exec.ExecuteTool(ctx, req.Name, req.Args, req.Timeout)
	if err != nil {
		return nil, err
	}
	return &ToolResponse{Result: out}, nil
}

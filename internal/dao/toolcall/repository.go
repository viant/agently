package toolcall

import (
	"context"
	read "github.com/viant/agently/internal/dao/toolcall/read"
	write "github.com/viant/agently/internal/dao/toolcall/write"
)

// API defines a backend-agnostic contract for tool call DAO.
type API interface {
	List(ctx context.Context, opts ...read.InputOption) ([]*read.ToolCallView, error)
	Patch(ctx context.Context, calls ...*write.ToolCall) (*write.Output, error)
}

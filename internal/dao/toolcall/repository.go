package toolcall

import (
	"context"

	write "github.com/viant/agently/pkg/agently/toolcall"
)

// API defines a backend-agnostic contract for tool call DAO.
type API interface {
	Patch(ctx context.Context, calls ...*write.ToolCall) (*write.Output, error)
}

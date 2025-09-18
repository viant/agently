package domain

import (
	"context"

	msgwrite "github.com/viant/agently/pkg/agently/message"
)

// Messages exposes message operations.
type Messages interface {
	// Patch upserts messages using DAO write model, returning DAO output.
	Patch(ctx context.Context, messages ...*msgwrite.Message) (*msgwrite.Output, error)
}

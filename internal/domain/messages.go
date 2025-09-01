package domain

import (
	"context"
	msgread "github.com/viant/agently/internal/dao/message/read"
	msgwrite "github.com/viant/agently/internal/dao/message/write"
)

// Messages exposes message operations.
type Messages interface {
	// Patch upserts messages using DAO write model, returning DAO output.
	Patch(ctx context.Context, messages ...*msgwrite.Message) (*msgwrite.Output, error)

	// List returns DAO read views using DAO read InputOptions.
	List(ctx context.Context, opts ...msgread.InputOption) ([]*msgread.MessageView, error)

	// GetTranscript returns a normalized transcript using DAO read views.
	GetTranscript(ctx context.Context, conversationID, turnID string, opts ...msgread.InputOption) ([]*msgread.MessageView, error)

	// Aggregated transcript remains available for one-call needs.
	GetTranscriptAggregated(ctx context.Context, conversationID, turnID string, opts TranscriptAggOptions) (*AggregatedTranscript, error)
}

package sql

import (
	"context"
	"net/http"
	"strings"
	"time"

	read2 "github.com/viant/agently/internal/dao/toolcall/read"
	"github.com/viant/agently/internal/dao/toolcall/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

func Register(ctx context.Context, dao *datly.Service) error {
	if err := read2.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := write.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

func (s *Service) List(ctx context.Context, opts ...read2.InputOption) ([]*read2.ToolCallView, error) {
	in := &read2.Input{}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		uri := strings.ReplaceAll(read2.PathByConversation, "{conversationId}", in.ConversationID)
		_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	}
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(read2.PathBase), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (s *Service) Patch(ctx context.Context, calls ...*write.ToolCall) (*write.Output, error) {
	in := &write.Input{ToolCalls: calls}
	out := &write.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, write.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Re-exports for ergonomics
type InputOption = read2.InputOption
type ToolCallView = read2.ToolCallView

func WithConversationID(id string) read2.InputOption { return read2.WithConversationID(id) }
func WithMessageID(id string) read2.InputOption      { return read2.WithMessageID(id) }
func WithMessageIDs(ids ...string) read2.InputOption { return read2.WithMessageIDs(ids...) }
func WithTurnID(id string) read2.InputOption         { return read2.WithTurnID(id) }
func WithOpID(op string) read2.InputOption           { return read2.WithOpID(op) }
func WithToolName(name string) read2.InputOption     { return read2.WithToolName(name) }
func WithStatus(status string) read2.InputOption     { return read2.WithStatus(status) }
func WithSince(ts time.Time) read2.InputOption       { return read2.WithSince(ts) }

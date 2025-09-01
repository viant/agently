package sql

import (
	"context"
	"net/http"
	"strings"
	"time"

	read2 "github.com/viant/agently/internal/dao/turn/read"
	write2 "github.com/viant/agently/internal/dao/turn/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

// Register components (to be invoked by parent module).
func Register(ctx context.Context, dao *datly.Service) error { return DefineComponent(ctx, dao) }

// List returns turns using input options.
func (s *Service) List(ctx context.Context, opts ...read2.InputOption) ([]*read2.TurnView, error) {
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
		var ret []*read2.TurnView
		for _, r := range out.Data {
			if r != nil {
				ret = append(ret, r)
			}
		}
		return ret, nil
	}
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(read2.PathBase), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	var ret []*read2.TurnView
	for _, r := range out.Data {
		if r != nil {
			ret = append(ret, r)
		}
	}
	return ret, nil
}

// Patch upserts turns via write component.
func (s *Service) Patch(ctx context.Context, turns ...*write2.Turn) (*write2.Output, error) {
	in := &write2.Input{Turns: turns}
	out := &write2.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, write2.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Re-exports for convenience and API ergonomics
type InputOption = read2.InputOption
type TurnView = read2.TurnView

func WithConversationID(id string) read2.InputOption { return read2.WithConversationID(id) }
func WithID(id string) read2.InputOption             { return read2.WithID(id) }
func WithIDs(ids ...string) read2.InputOption        { return read2.WithIDs(ids...) }
func WithStatus(status string) read2.InputOption     { return read2.WithStatus(status) }

// WithSince re-exports read.WithSince
func WithSince(ts time.Time) read2.InputOption { return read2.WithSince(ts) }

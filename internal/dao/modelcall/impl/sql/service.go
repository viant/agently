package sql

import (
	"context"
	"net/http"
	"strings"
	"time"

	read2 "github.com/viant/agently/internal/dao/modelcall/read"
	"github.com/viant/agently/internal/dao/modelcall/write"
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

func (s *Service) List(ctx context.Context, opts ...read2.InputOption) ([]*read2.ModelCallView, error) {
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

func (s *Service) Patch(ctx context.Context, calls ...*write.ModelCall) (*write.Output, error) {
	in := &write.Input{ModelCalls: calls}
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

// Ergonomic re-exports
type InputOption = read2.InputOption
type ModelCallView = read2.ModelCallView

func WithConversationID(id string) read2.InputOption { return read2.WithConversationID(id) }
func WithMessageID(id string) read2.InputOption      { return read2.WithMessageID(id) }
func WithMessageIDs(ids ...string) read2.InputOption { return read2.WithMessageIDs(ids...) }
func WithTurnID(id string) read2.InputOption         { return read2.WithTurnID(id) }
func WithProvider(p string) read2.InputOption        { return read2.WithProvider(p) }
func WithModel(m string) read2.InputOption           { return read2.WithModel(m) }
func WithModelKind(k string) read2.InputOption       { return read2.WithModelKind(k) }
func WithSince(ts time.Time) read2.InputOption       { return read2.WithSince(ts) }

package sql

import (
	"context"
	"net/http"
	"strings"

	"github.com/viant/agently/internal/dao/conversation/read"
	"github.com/viant/agently/internal/dao/conversation/write"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

// Service provides minimal DAO operations for v2 conversations/messages using Datly.
type Service struct {
	dao *datly.Service
}

// GetConversation fetches conversation meta by id.

// GetConversations returns conversations using optional filters.
func (s *Service) GetConversations(ctx context.Context, opts ...read.ConversationInputOption) ([]*read.ConversationView, error) {
	in := &read.ConversationInput{}
	for _, opt := range opts {
		opt(in)
	}
	output := &read.ConversationOutput{}
	// Use path with id when provided for precise routing; otherwise base path with predicates.
	if in.Has != nil && in.Has.Id && in.Id != "" {
		uri := strings.ReplaceAll(read.ConversationPathURI, "{id}", in.Id)
		_, err := s.dao.Operate(ctx, datly.WithOutput(output), datly.WithURI(uri), datly.WithInput(in))
		if err != nil {
			return nil, err
		}
		return output.Data, nil
	}

	_, err := s.dao.Operate(ctx, datly.WithOutput(output), datly.WithURI(read.ConversationBasePathURI), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	return output.Data, nil
}

// FindConversations returns conversations filtered by summary (contains).
// Backward-compat thin wrappers (optional). Consider removing after adoption.
// GetConversation returns 0..1 record by id.
func (s *Service) GetConversation(ctx context.Context, convID string) (*read.ConversationView, error) {
	rows, err := s.GetConversations(ctx, read.WithID(convID))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// New constructs the v2 Service and registers components.
func New(ctx context.Context, connector *view.Connector, options ...repository.Option) (*Service, error) {
	dao, err := datly.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	if err := dao.AddConnectors(ctx, connector); err != nil {
		return nil, err
	}
	ret := &Service{dao: dao}
	if err := ret.init(ctx); err != nil {
		return nil, err
	}
	return ret, nil
}

func NewService(ctx context.Context, dao *datly.Service) *Service {
	ret := &Service{dao: dao}
	if err := ret.init(ctx); err != nil {
		return nil
	}
	return ret
}

func Register(ctx context.Context, dao *datly.Service) error {
	if err := read.DefineConversationComponent(ctx, dao); err != nil {
		return err
	}
	// Register rich conversation (transcript + usage) component from pkg/agently
	if err := agconv.DefineConversationComponent(ctx, dao); err != nil {
		return err
	}

	if _, err := write.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

func (s *Service) init(ctx context.Context) error {
	if err := read.DefineConversationComponent(ctx, s.dao); err != nil {
		return err
	}
	// Register rich conversation (transcript + usage) component from pkg/agently
	if err := agconv.DefineConversationComponent(ctx, s.dao); err != nil {
		return err
	}
	if _, err := write.DefineComponent(ctx, s.dao); err != nil {
		return err
	}
	return nil
}

// GetConversationRich returns the rich conversation view (metadata + transcript + usage)
// using the generated component in pkg/agently/conversation.
func (s *Service) GetConversationRich(ctx context.Context, id string) (*agconv.ConversationView, error) {
	in := &agconv.ConversationInput{Id: id}
	out := &agconv.ConversationOutput{}
	uri := strings.ReplaceAll(agconv.ConversationPathURI, "{id}", id)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	return out.Data[0], nil
}

// PatchConversations upserts conversations using write component.
func (s *Service) PatchConversations(ctx context.Context, conversations ...*write.Conversation) (*write.Output, error) {
	input := &write.Input{Conversations: conversations}
	out := &write.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, write.PathURI)),
		datly.WithInput(input),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// WithID is a thin wrapper returning a read.ConversationInputOption that filters by id.
func WithID(id string) read.ConversationInputOption { return read.WithID(id) }

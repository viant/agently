package sql

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/viant/agently/internal/dao/message/impl/shared"
	read2 "github.com/viant/agently/internal/dao/message/read"
	"github.com/viant/agently/internal/dao/message/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

// Register components (to be invoked by parent module).
func Register(ctx context.Context, dao *datly.Service) error {
	if err := read2.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := write.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

// List returns messages using input options.
func (s *Service) List(ctx context.Context, opts ...read2.InputOption) ([]*read2.MessageView, error) {
	in := &read2.Input{}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	// prefer path with conversation when provided (predicates still apply)
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

// GetTranscript returns transcript for a given conversation and turn.
// Includes roles: user, assistant, tool. Excludes control and interim by default.
// Tool messages are de-duplicated by op_id keeping the latest attempt.
func (s *Service) GetTranscript(ctx context.Context, conversationID, turnID string, opts ...read2.InputOption) ([]*read2.MessageView, error) {
	in := &read2.Input{ConversationID: conversationID, TurnID: turnID, Has: &read2.Has{ConversationID: true, TurnID: true}}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	uri := strings.ReplaceAll(read2.PathByConversation, "{conversationId}", conversationID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	// Normalize transcript with shared logic
	return shared.BuildTranscript(out.Data, true), nil
}

// GetConversation returns assistant/user messages for a conversation (no tool messages).
// Interim messages are excluded by default. Additional filters can be provided via InputOption.
func (s *Service) GetConversation(ctx context.Context, conversationID string, opts ...read2.InputOption) ([]*read2.MessageView, error) {
	in := &read2.Input{ConversationID: conversationID, Has: &read2.Has{ConversationID: true}}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	uri := strings.ReplaceAll(read2.PathByConversation, "{conversationId}", conversationID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err != nil {
		return nil, err
	}

	// filter to user/assistant only, exclude control and interim by default
	var filtered []*read2.MessageView
	for _, m := range out.Data {
		if m == nil {
			continue
		}
		if m.Type == "control" {
			continue
		}
		if m.Interim != nil && *m.Interim == 1 {
			continue
		}
		if m.Role == "user" || m.Role == "assistant" {
			filtered = append(filtered, m)
		}
	}
	// sort by created_at asc
	sort.SliceStable(filtered, func(i, j int) bool {
		li, lj := filtered[i], filtered[j]
		if li.CreatedAt != nil && lj.CreatedAt != nil {
			return li.CreatedAt.Before(*lj.CreatedAt)
		}
		return i < j
	})
	return filtered, nil
}

// Patch upserts messages via write component
func (s *Service) Patch(ctx context.Context, messages ...*write.Message) (*write.Output, error) {
	in := &write.Input{Messages: messages}
	out := &write.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", write.PathURI)),
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
type MessageView = read2.MessageView

func WithConversationID(id string) read2.InputOption { return read2.WithConversationID(id) }
func WithID(id string) read2.InputOption             { return read2.WithID(id) }
func WithIDs(ids ...string) read2.InputOption        { return read2.WithIDs(ids...) }
func WithRole(role string) read2.InputOption         { return read2.WithRoles(role) }
func WithType(typ string) read2.InputOption          { return read2.WithType(typ) }
func WithInterim(values ...int) read2.InputOption    { return read2.WithInterim(values...) }
func WithElicitationID(id string) read2.InputOption  { return read2.WithElicitationID(id) }
func WithTurnID(id string) read2.InputOption         { return read2.WithTurnID(id) }
func WithSince(ts time.Time) read2.InputOption       { return read2.WithSince(ts) }

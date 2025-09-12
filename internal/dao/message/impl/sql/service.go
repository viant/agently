package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/internal/dao/message/impl/shared"
	read "github.com/viant/agently/internal/dao/message/read"
	"github.com/viant/agently/internal/dao/message/write"
	pldaoRead "github.com/viant/agently/internal/dao/payload/read"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

// Register components (to be invoked by parent module).
func Register(ctx context.Context, dao *datly.Service) error {
	if err := read.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := write.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

// List returns messages using input options.
func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.MessageView, error) {
	in := &read.Input{}
	for _, opt := range opts {
		opt(in)
	}
	out := &read.Output{}
	// prefer path with conversation when provided (predicates still apply)
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		uri := strings.ReplaceAll(read.PathByConversation, "{conversationId}", in.ConversationID)
		_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	}
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(read.PathBase), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}

// GetTranscript returns transcript for a given conversation and turn.
// Includes roles: user, assistant, tool. Excludes control and interim by default.
// Tool messages are de-duplicated by op_id keeping the latest attempt.
func (s *Service) GetTranscript(ctx context.Context, conversationID string, opts ...read.InputOption) ([]*read.MessageView, error) {
	in := &read.Input{ConversationID: conversationID, Has: &read.Has{ConversationID: true}}
	for _, opt := range opts {
		opt(in)
	}
	out := &read.Output{}
	uri := strings.ReplaceAll(read.PathByConversation, "{conversationId}", conversationID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err == nil && out.Status.Status == "error" {
		err = fmt.Errorf("transcript error: %s", out.Status.Message)
	}
	if err != nil {
		return nil, err
	}
	// Normalize transcript with shared logic
	rows := shared.BuildTranscript(out.Data, false)

	//TODO move to the hooks
	for _, v := range rows {
		if v == nil || v.ElicitationID == nil || *v.ElicitationID == "" {
			continue
		}
		pout := &pldaoRead.Output{}
		_, perr := s.dao.Operate(ctx,
			datly.WithURI(pldaoRead.PathBase+"?id="+*v.ElicitationID),
			datly.WithOutput(pout),
			datly.WithInput(&pldaoRead.Input{Id: *v.ElicitationID, Has: &pldaoRead.Has{Id: true}}),
		)
		if perr != nil || len(pout.Data) == 0 || pout.Data[0] == nil {
			continue
		}
		pv := pout.Data[0]
		if pv.InlineBody != nil {
			s := string(*pv.InlineBody)
			v.ElicitationJSON = &s
		} else if pv.Preview != nil {
			s := string(*pv.Preview)
			v.ElicitationJSON = &s
		}
		// decode typed elicitation when available
		if v.ElicitationJSON != nil && *v.ElicitationJSON != "" {
			var el plan.Elicitation
			if json.Unmarshal([]byte(*v.ElicitationJSON), &el) == nil {
				v.Elicitation = &el
			}
		}
	}

	// Optional: slice by SinceID (inclusive) when provided.
	if in.SinceID != "" {
		start := -1
		for i, v := range rows {
			if v != nil && v.Id == in.SinceID {
				start = i
				break
			}
		}
		if start >= 0 {
			rows = rows[start:]
		} else {
			rows = rows[:0]
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		li, lj := rows[i], rows[j]
		if li.CreatedAt != nil && lj.CreatedAt != nil {
			return li.CreatedAt.Before(*lj.CreatedAt)
		}
		return i < j
	})
	return rows, nil
}

// GetConversation returns assistant/user messages for a conversation (no tool messages).
// Interim messages are excluded by default. Additional filters can be provided via InputOption.
func (s *Service) GetConversation(ctx context.Context, conversationID string, opts ...read.InputOption) ([]*read.MessageView, error) {
	in := &read.Input{ConversationID: conversationID, Has: &read.Has{ConversationID: true}}
	for _, opt := range opts {
		opt(in)
	}
	out := &read.Output{}
	uri := strings.ReplaceAll(read.PathByConversation, "{conversationId}", conversationID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err != nil {
		return nil, err
	}

	// filter to user/assistant only, exclude control and interim by default
	var filtered []*read.MessageView
	for _, m := range out.Data {
		if m == nil {
			continue
		}
		if m.Type == "control" {
			continue
		}
		if m.IsInterim() {
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
type InputOption = read.InputOption
type MessageView = read.MessageView

func WithConversationID(id string) read.InputOption { return read.WithConversationID(id) }
func WithID(id string) read.InputOption             { return read.WithID(id) }
func WithIDs(ids ...string) read.InputOption        { return read.WithIDs(ids...) }
func WithRole(role string) read.InputOption         { return read.WithRoles(role) }
func WithType(typ string) read.InputOption          { return read.WithType(typ) }
func WithInterim(values ...int) read.InputOption    { return read.WithInterim(values...) }
func WithTurnID(id string) read.InputOption         { return read.WithTurnID(id) }
func WithSince(ts time.Time) read.InputOption       { return read.WithSince(ts) }

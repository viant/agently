package memory

import (
	"context"
	"sync"
	"time"

	api "github.com/viant/agently/internal/dao/turn"
	read "github.com/viant/agently/internal/dao/turn/read"
	write "github.com/viant/agently/pkg/agently/turn"
)

// Service is an in-memory implementation of the Turn API.
type Service struct {
	mu     sync.RWMutex
	turns  map[string]*read.TurnView
	byConv map[string][]*read.TurnView
}

func New() *Service {
	return &Service{turns: map[string]*read.TurnView{}, byConv: map[string][]*read.TurnView{}}
}

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.TurnView, error) {
	in := &read.Input{}
	for _, opt := range opts {
		opt(in)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*read.TurnView
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		for _, v := range s.byConv[in.ConversationID] {
			if match(v, in) {
				out = append(out, clone(v))
			}
		}
		return out, nil
	}
	for _, v := range s.turns {
		if match(v, in) {
			out = append(out, clone(v))
		}
	}
	return out, nil
}

func (s *Service) Patch(ctx context.Context, turns ...*write.Turn) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, rec := range turns {
		if rec == nil {
			continue
		}
		v, ok := s.turns[rec.Id]
		if !ok {
			v = &read.TurnView{Id: rec.Id, ConversationID: rec.ConversationID, Status: rec.Status}
			v.CreatedAt = &now
		} else {
			v.Status = rec.Status
		}
		s.turns[rec.Id] = v
		list := s.byConv[v.ConversationID]
		found := false
		for i := range list {
			if list[i].Id == v.Id {
				list[i] = v
				found = true
				break
			}
		}
		if !found {
			s.byConv[v.ConversationID] = append(list, v)
		} else {
			s.byConv[v.ConversationID] = list
		}
	}
	return &write.Output{Data: turns}, nil
}

func match(v *read.TurnView, in *read.Input) bool {
	if in.Has == nil {
		return true
	}
	if in.Has.ConversationID && in.ConversationID != v.ConversationID {
		return false
	}
	if in.Has.Id && in.Id != v.Id {
		return false
	}
	if in.Has.Ids && len(in.Ids) > 0 {
		ok := false
		for _, id := range in.Ids {
			if id == v.Id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if in.Has.Status && in.Status != v.Status {
		return false
	}
	if in.Has.Since && in.Since != nil && v.CreatedAt != nil && v.CreatedAt.Before(*in.Since) {
		return false
	}
	return true
}

func clone(v *read.TurnView) *read.TurnView {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

var _ api.API = (*Service)(nil)

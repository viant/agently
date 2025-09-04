package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	api "github.com/viant/agently/internal/dao/message"
	shared "github.com/viant/agently/internal/dao/message/impl/shared"
	read "github.com/viant/agently/internal/dao/message/read"
	write "github.com/viant/agently/internal/dao/message/write"
)

// Service is an in-memory implementation of the Message API.
type Service struct {
	mu       sync.RWMutex
	messages map[string]*read.MessageView   // by id
	byConv   map[string][]*read.MessageView // conversationID -> ordered slice
}

func New() *Service {
	return &Service{messages: map[string]*read.MessageView{}, byConv: map[string][]*read.MessageView{}}
}

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.MessageView, error) {
	in := &read.Input{}
	for _, opt := range opts {
		opt(in)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*read.MessageView
	// If conversation provided, iterate its slice for performance
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		for _, mv := range s.byConv[in.ConversationID] {
			if matchFilter(mv, in) {
				out = append(out, clone(mv))
			}
		}
		return out, nil
	}
	for _, mv := range s.messages {
		if matchFilter(mv, in) {
			out = append(out, clone(mv))
		}
	}
	return out, nil
}

func (s *Service) GetTranscript(ctx context.Context, conversationID, turnID string, opts ...read.InputOption) ([]*read.MessageView, error) {
	in := &read.Input{ConversationID: conversationID, TurnID: turnID, Has: &read.Has{ConversationID: true, TurnID: true}}
	for _, opt := range opts {
		opt(in)
	}
	rows, err := s.List(ctx, func(_ *read.Input) {})
	if err != nil {
		return nil, err
	}
	// restrict to conv/turn after list
	var filtered []*read.MessageView
	for _, m := range rows {
		if m.ConversationID != conversationID {
			continue
		}
		if m.TurnID == nil || *m.TurnID != turnID {
			continue
		}
		filtered = append(filtered, m)
	}
	return shared.BuildTranscript(filtered, true), nil
}

func (s *Service) GetConversation(ctx context.Context, conversationID string, opts ...read.InputOption) ([]*read.MessageView, error) {
	in := &read.Input{ConversationID: conversationID, Has: &read.Has{ConversationID: true}}
	for _, opt := range opts {
		opt(in)
	}
	rows, err := s.List(ctx, func(_ *read.Input) {})
	if err != nil {
		return nil, err
	}
	var result []*read.MessageView
	for _, m := range rows {
		if m.ConversationID == conversationID {
			result = append(result, m)
		}
	}
	// Keep returned order deterministic (created_at asc if available)
	// Note: GetConversation is not the transcript; simple created_at ordering is sufficient
	if len(result) > 1 {
		// shallow sort
		for i := 0; i < len(result)-1; i++ {
			for j := i + 1; j < len(result); j++ {
				li, lj := result[i], result[j]
				if li.CreatedAt != nil && lj.CreatedAt != nil && li.CreatedAt.After(*lj.CreatedAt) {
					result[i], result[j] = result[j], result[i]
				}
			}
		}
	}
	return result, nil
}

func (s *Service) Patch(ctx context.Context, messages ...*write.Message) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, rec := range messages {
		if rec == nil {
			continue
		}
		mv, ok := s.messages[rec.Id]
		if !ok {
			mv = &read.MessageView{Id: rec.Id, ConversationID: rec.ConversationID}
			created := now
			mv.CreatedAt = &created
		}
		// Apply fields honoring Has markers
		if rec.Has != nil {
			if rec.Has.ConversationID {
				mv.ConversationID = rec.ConversationID
			}
			if rec.Has.TurnID {
				mv.TurnID = rec.TurnID
			}
			if rec.Has.Sequence {
				mv.Sequence = rec.Sequence
			}
			if rec.Has.Role {
				mv.Role = rec.Role
			}
			if rec.Has.Type {
				mv.Type = rec.Type
			}
			if rec.Has.Content {
				mv.Content = rec.Content
			}
			if rec.Has.ElicitationID {
				mv.ElicitationID = rec.ElicitationID
			}
			if rec.Has.Interim {
				mv.Interim = rec.Interim
			}
			if rec.Has.ToolName {
				mv.ToolName = rec.ToolName
			}
		}
		s.messages[rec.Id] = mv
		// update index by conversation
		found := false
		list := s.byConv[mv.ConversationID]
		for i := range list {
			if list[i].Id == mv.Id {
				list[i] = mv
				found = true
				break
			}
		}
		if !found {
			s.byConv[mv.ConversationID] = append(list, mv)
		} else {
			s.byConv[mv.ConversationID] = list
		}
	}
	return &write.Output{Data: messages}, nil
}

func matchFilter(mv *read.MessageView, in *read.Input) bool {
	if in.Has == nil {
		return true
	}
	if in.Has.Id && in.Id != "" && mv.Id != in.Id {
		return false
	}
	if in.Has.Ids && len(in.Ids) > 0 {
		ok := false
		for _, id := range in.Ids {
			if id == mv.Id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if in.Has.ConversationID && in.ConversationID != mv.ConversationID {
		return false
	}
	if in.Has.TurnID && (mv.TurnID == nil || *mv.TurnID != in.TurnID) {
		return false
	}
	if in.Has.Roles && len(mv.Role) != len(in.Roles) {
		return false
	}
	if in.Has.Type && mv.Type != in.Type {
		return false
	}
	if in.Has.Interim && len(in.Interim) > 0 {
		v := 0
		if mv.Interim != nil {
			v = *mv.Interim
		}
		ok := false
		for _, i := range in.Interim {
			if i == v {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if in.Has.Since && in.Since != nil && mv.CreatedAt != nil {
		if mv.CreatedAt.Before(*in.Since) {
			return false
		}
	}
	if in.Has.ElicitationID && (mv.ElicitationID == nil || *mv.ElicitationID != in.ElicitationID) {
		return false
	}
	// simple contains filters on content if needed could be added later
	_ = strings.Contains
	return true
}

func clone(v *read.MessageView) *read.MessageView {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

var _ api.API = (*Service)(nil)

package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	conv "github.com/viant/agently/internal/dao/conversation"
	read2 "github.com/viant/agently/internal/dao/conversation/read"
	"github.com/viant/agently/pkg/agently/conversation/write"
)

// Service is an in-memory implementation of the conversation DAO API.
type Service struct {
	mu    sync.RWMutex
	items map[string]*read2.ConversationView
}

func New() *Service { return &Service{items: map[string]*read2.ConversationView{}} }

func (s *Service) GetConversations(ctx context.Context, opts ...read2.ConversationInputOption) ([]*read2.ConversationView, error) {
	in := &read2.ConversationInput{}
	for _, opt := range opts {
		opt(in)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*read2.ConversationView
	// fast path by ID
	if in.Has != nil && in.Has.Id && in.Id != "" {
		if v, ok := s.items[in.Id]; ok {
			out = append(out, cloneView(v))
		}
		return out, nil
	}
	for _, v := range s.items {
		if matchFilter(v, in) {
			out = append(out, cloneView(v))
		}
	}
	return out, nil
}

func (s *Service) GetConversation(ctx context.Context, convID string) (*read2.ConversationView, error) {
	rows, err := s.GetConversations(ctx, read2.WithID(convID))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (s *Service) PatchConversations(ctx context.Context, conversations ...*write.Conversation) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, rec := range conversations {
		if rec == nil {
			continue
		}
		cur, ok := s.items[rec.Id]
		if !ok {
			cur = &read2.ConversationView{Id: rec.Id}
			created := now
			lastAct := now
			cur.CreatedAt = &created
			cur.LastActivity = &lastAct
		} else {
			lastAct := now
			cur.LastActivity = &lastAct
		}
		if rec.Has != nil {
			if rec.Has.Summary {
				cur.Summary = rec.Summary
			}
			if rec.Has.AgentName {
				// write struct has AgentName string; map to pointer
				name := rec.AgentName
				cur.AgentName = &name
			}
			if rec.Has.UsageInputTokens {
				v := rec.UsageInputTokens
				cur.UsageInputTokens = &v
			}
			if rec.Has.UsageOutputTokens {
				v := rec.UsageOutputTokens
				cur.UsageOutputTokens = &v
			}
			if rec.Has.UsageEmbeddingTokens {
				v := rec.UsageEmbeddingTokens
				cur.UsageEmbeddingTokens = &v
			}
			if rec.Has.CreatedByUserID {
				cur.CreatedByUserID = rec.CreatedByUserID
			}
		}
		s.items[rec.Id] = cur
	}
	return &write.Output{Data: conversations}, nil
}

// Helper: filters based on input
func matchFilter(v *read2.ConversationView, in *read2.ConversationInput) bool {
	if in.Has == nil {
		return true
	}
	if in.Has.Summary && (v.Summary == nil || !strings.Contains(strings.ToLower(*v.Summary), strings.ToLower(in.Summary))) {
		return false
	}
	if in.Has.Title && (v.Title == nil || !strings.Contains(strings.ToLower(*v.Title), strings.ToLower(in.Title))) {
		return false
	}
	if in.Has.AgentName && (v.AgentName == nil || !strings.Contains(strings.ToLower(*v.AgentName), strings.ToLower(in.AgentName))) {
		return false
	}
	if in.Has.AgentID && (v.AgentID == nil || *v.AgentID != in.AgentID) {
		return false
	}
	if in.Has.Visibility && (v.Visibility == nil || *v.Visibility != in.Visibility) {
		return false
	}
	if in.Has.TenantID && (v.TenantID == nil || *v.TenantID != in.TenantID) {
		return false
	}
	if in.Has.CreatedByUserID && (v.CreatedByUserID == nil || *v.CreatedByUserID != in.CreatedByUserID) {
		return false
	}
	if in.Has.Archived && len(in.Archived) > 0 {
		if v.Archived == nil {
			return false
		}
		ok := false
		for _, a := range in.Archived {
			if *v.Archived == a {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func cloneView(v *read2.ConversationView) *read2.ConversationView {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// Ensure memory.Service implements the same API as SQL service.
var _ conv.API = (*Service)(nil)

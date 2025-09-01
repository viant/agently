package memory

import (
	"context"
	"sync"
	"time"

	api "github.com/viant/agently/internal/dao/usage"
	read "github.com/viant/agently/internal/dao/usage/read"
	write "github.com/viant/agently/internal/dao/usage/write"
)

// Service is an in-memory implementation of the Usage API.
// It supports List by aggregating seeded model call stats and Patch for updating totals per conversation.
type Service struct {
	mu     sync.RWMutex
	calls  []modelCall
	totals map[string]UsageTotals // conversationID -> totals
}

type modelCall struct {
	ConversationID   string
	Provider         string
	Model            string
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	Cost             float64
	CacheHit         int
	StartedAt        *time.Time
	CompletedAt      *time.Time
}

type UsageTotals struct{ Input, Output, Embedding int }

func New() *Service { return &Service{totals: map[string]UsageTotals{}} }

// SeedCall adds a synthetic model call used for aggregation in tests.
func (s *Service) SeedCall(c modelCall) { s.mu.Lock(); s.calls = append(s.calls, c); s.mu.Unlock() }

func (s *Service) List(ctx context.Context, in read.Input) ([]*read.UsageView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// aggregate per conversation/provider/model
	type key struct{ Conv, Prov, Mod string }
	agg := map[key]*read.UsageView{}
	for _, c := range s.calls {
		if in.Has != nil && in.Has.ConversationID && in.ConversationID != c.ConversationID {
			continue
		}
		if in.Has != nil && in.Has.Provider && in.Provider != c.Provider {
			continue
		}
		if in.Has != nil && in.Has.Model && in.Model != c.Model {
			continue
		}
		if in.Has != nil && in.Has.Since && in.Since != nil && c.StartedAt != nil && c.StartedAt.Before(*in.Since) {
			continue
		}
		k := key{c.ConversationID, c.Provider, c.Model}
		v, ok := agg[k]
		if !ok {
			v = &read.UsageView{ConversationID: c.ConversationID, Provider: c.Provider, Model: c.Model}
			agg[k] = v
		}
		// sum fields
		sum := func(p **int, add int) {
			if *p == nil {
				z := 0
				*p = &z
			}
			**p += add
		}
		sum(&v.TotalTokens, c.TotalTokens)
		sum(&v.TotalPromptTokens, c.PromptTokens)
		sum(&v.TotalCompletionTokens, c.CompletionTokens)
		if v.CallsCount == nil {
			z := 0
			v.CallsCount = &z
		}
		*v.CallsCount += 1
		if c.CacheHit == 1 {
			if v.CachedCalls == nil {
				z := 0
				v.CachedCalls = &z
			}
			*v.CachedCalls += 1
		}
		if c.Cost > 0 {
			if v.TotalCost == nil {
				z := 0.0
				v.TotalCost = &z
			}
			*v.TotalCost += c.Cost
		}
		if c.StartedAt != nil {
			if v.FirstCallAt == nil || c.StartedAt.Before(*v.FirstCallAt) {
				v.FirstCallAt = c.StartedAt
			}
		}
		if c.CompletedAt != nil {
			if v.LastCallAt == nil || c.CompletedAt.After(*v.LastCallAt) {
				v.LastCallAt = c.CompletedAt
			}
		}
	}
	// build output
	var out []*read.UsageView
	for _, v := range agg {
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) Patch(ctx context.Context, usages ...*write.Usage) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range usages {
		if u == nil {
			continue
		}
		t := s.totals[u.Id]
		if u.Has != nil {
			if u.Has.UsageInputTokens {
				t.Input = u.UsageInputTokens
			}
			if u.Has.UsageOutputTokens {
				t.Output = u.UsageOutputTokens
			}
			if u.Has.UsageEmbeddingTokens {
				t.Embedding = u.UsageEmbeddingTokens
			}
		}
		s.totals[u.Id] = t
	}
	return &write.Output{Data: usages}, nil
}

var _ api.API = (*Service)(nil)

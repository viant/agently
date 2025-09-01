package memory

import (
	"context"
	"strings"
	"sync"

	api "github.com/viant/agently/internal/dao/modelcall"
	read "github.com/viant/agently/internal/dao/modelcall/read"
	write "github.com/viant/agently/internal/dao/modelcall/write"
)

// Service is an in-memory implementation of the ModelCall API.
type Service struct {
	mu     sync.RWMutex
	items  map[string]*read.ModelCallView // by message_id
	byConv map[string][]*read.ModelCallView
}

func New() *Service {
	return &Service{items: map[string]*read.ModelCallView{}, byConv: map[string][]*read.ModelCallView{}}
}

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.ModelCallView, error) {
	in := &read.Input{}
	for _, opt := range opts {
		opt(in)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*read.ModelCallView
	// If conversationID filter supplied, scan byConv; else scan all
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		for _, v := range s.byConv[in.ConversationID] {
			if match(v, in) {
				out = append(out, clone(v))
			}
		}
		return out, nil
	}
	for _, v := range s.items {
		if match(v, in) {
			out = append(out, clone(v))
		}
	}
	return out, nil
}

func (s *Service) Patch(ctx context.Context, calls ...*write.ModelCall) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range calls {
		if rec == nil {
			continue
		}
		v, ok := s.items[rec.MessageID]
		if !ok {
			v = &read.ModelCallView{MessageID: rec.MessageID}
		}
		if rec.Has != nil {
			if rec.Has.TurnID {
				v.TurnID = rec.TurnID
			}
			if rec.Has.Provider {
				v.Provider = rec.Provider
			}
			if rec.Has.Model {
				v.Model = rec.Model
			}
			if rec.Has.ModelKind {
				v.ModelKind = rec.ModelKind
			}
			if rec.Has.PromptTokens {
				v.PromptTokens = rec.PromptTokens
			}
			if rec.Has.CompletionTokens {
				v.CompletionTokens = rec.CompletionTokens
			}
			if rec.Has.TotalTokens {
				v.TotalTokens = rec.TotalTokens
			}
			if rec.Has.FinishReason {
				v.FinishReason = rec.FinishReason
			}
			if rec.Has.CacheHit {
				v.CacheHit = rec.CacheHit
			}
			if rec.Has.CacheKey {
				v.CacheKey = rec.CacheKey
			}
			if rec.Has.StartedAt {
				v.StartedAt = rec.StartedAt
			}
			if rec.Has.CompletedAt {
				v.CompletedAt = rec.CompletedAt
			}
			if rec.Has.LatencyMS {
				v.LatencyMS = rec.LatencyMS
			}
			if rec.Has.Cost {
				v.Cost = rec.Cost
			}
			if rec.Has.TraceID {
				v.TraceID = rec.TraceID
			}
			if rec.Has.SpanID {
				v.SpanID = rec.SpanID
			}
			if rec.Has.RequestPayloadID {
				v.RequestPayloadID = rec.RequestPayloadID
			}
			if rec.Has.ResponsePayloadID {
				v.ResponsePayloadID = rec.ResponsePayloadID
			}
		}
		s.items[rec.MessageID] = v
		// cannot derive conversationID from model_calls alone; keep byConv empty unless needed
	}
	return &write.Output{Data: calls}, nil
}

func match(v *read.ModelCallView, in *read.Input) bool {
	if in.Has == nil {
		return true
	}
	if in.Has.MessageID && in.MessageID != v.MessageID {
		return false
	}
	if in.Has.MessageIDs && len(in.MessageIDs) > 0 {
		ok := false
		for _, id := range in.MessageIDs {
			if id == v.MessageID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if in.Has.TurnID && (v.TurnID == nil || *v.TurnID != in.TurnID) {
		return false
	}
	if in.Has.Provider && !strings.EqualFold(v.Provider, in.Provider) {
		return false
	}
	if in.Has.Model && v.Model != in.Model {
		return false
	}
	if in.Has.ModelKind && v.ModelKind != in.ModelKind {
		return false
	}
	if in.Has.Since && in.Since != nil && v.StartedAt != nil && v.StartedAt.Before(*in.Since) {
		return false
	}
	return true
}

func clone(v *read.ModelCallView) *read.ModelCallView {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

var _ api.API = (*Service)(nil)

package memory

import (
	"context"
	"sync"

	api "github.com/viant/agently/internal/dao/modelcall"
	read "github.com/viant/agently/internal/dao/modelcall/read"
	write "github.com/viant/agently/pkg/agently/modelcall"
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

var _ api.API = (*Service)(nil)

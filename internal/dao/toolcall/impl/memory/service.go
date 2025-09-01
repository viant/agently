package memory

import (
	"context"
	"strings"
	"sync"

	api "github.com/viant/agently/internal/dao/toolcall"
	read "github.com/viant/agently/internal/dao/toolcall/read"
	write "github.com/viant/agently/internal/dao/toolcall/write"
)

// Service is an in-memory implementation of the ToolCall API.
type Service struct {
	mu    sync.RWMutex
	items map[string]*read.ToolCallView // keyed by message_id
}

func New() *Service { return &Service{items: map[string]*read.ToolCallView{}} }

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.ToolCallView, error) {
	in := &read.Input{}
	for _, opt := range opts {
		opt(in)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*read.ToolCallView
	for _, v := range s.items {
		if match(v, in) {
			out = append(out, clone(v))
		}
	}
	return out, nil
}

func (s *Service) Patch(ctx context.Context, calls ...*write.ToolCall) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range calls {
		if rec == nil {
			continue
		}
		v, ok := s.items[rec.MessageID]
		if !ok {
			v = &read.ToolCallView{MessageID: rec.MessageID}
		}
		if rec.Has != nil {
			if rec.Has.TurnID {
				v.TurnID = rec.TurnID
			}
			if rec.Has.OpID {
				v.OpID = rec.OpID
			}
			if rec.Has.Attempt {
				v.Attempt = rec.Attempt
			}
			if rec.Has.ToolName {
				v.ToolName = rec.ToolName
			}
			if rec.Has.ToolKind {
				v.ToolKind = rec.ToolKind
			}
			if rec.Has.CapabilityTags {
				v.CapabilityTags = rec.CapabilityTags
			}
			if rec.Has.ResourceURIs {
				v.ResourceURIs = rec.ResourceURIs
			}
			if rec.Has.Status {
				v.Status = rec.Status
			}
			if rec.Has.RequestSnapshot {
				v.RequestSnapshot = rec.RequestSnapshot
			}
			if rec.Has.RequestHash {
				v.RequestHash = rec.RequestHash
			}
			if rec.Has.ResponseSnapshot {
				v.ResponseSnapshot = rec.ResponseSnapshot
			}
			if rec.Has.ErrorCode {
				v.ErrorCode = rec.ErrorCode
			}
			if rec.Has.ErrorMessage {
				v.ErrorMessage = rec.ErrorMessage
			}
			if rec.Has.Retriable {
				v.Retriable = rec.Retriable
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
		}
		s.items[rec.MessageID] = v
	}
	return &write.Output{Data: calls}, nil
}

func match(v *read.ToolCallView, in *read.Input) bool {
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
	if in.Has.OpID && v.OpID != in.OpID {
		return false
	}
	if in.Has.ToolName && !strings.EqualFold(v.ToolName, in.ToolName) {
		return false
	}
	if in.Has.Status && v.Status != in.Status {
		return false
	}
	return true
}

func clone(v *read.ToolCallView) *read.ToolCallView {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

var _ api.API = (*Service)(nil)

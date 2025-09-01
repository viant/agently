package memory

import (
	"bytes"
	"compress/gzip"
	"context"
	"sync"
	"time"

	api "github.com/viant/agently/internal/dao/payload"
	read "github.com/viant/agently/internal/dao/payload/read"
	write "github.com/viant/agently/internal/dao/payload/write"
)

// Service is an in-memory implementation of the Payload API.
type Service struct {
	mu      sync.RWMutex
	payload map[string]*read.PayloadView
}

func New() *Service { return &Service{payload: map[string]*read.PayloadView{}} }

func (s *Service) List(ctx context.Context, opts ...read.InputOption) ([]*read.PayloadView, error) {
	in := &read.Input{}
	for _, opt := range opts {
		opt(in)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*read.PayloadView
	for _, v := range s.payload {
		if match(v, in) {
			c := clone(v)
			// Auto-decompress inline body when compressed
			if c != nil && c.Compression == "gzip" && c.InlineBody != nil {
				gr, err := gzip.NewReader(bytes.NewReader(*c.InlineBody))
				if err == nil {
					var buf bytes.Buffer
					_, _ = buf.ReadFrom(gr)
					_ = gr.Close()
					b := buf.Bytes()
					c.InlineBody = &b
				}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Service) Patch(ctx context.Context, payloads ...*write.Payload) (*write.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range payloads {
		if rec == nil {
			continue
		}
		v, ok := s.payload[rec.Id]
		if !ok {
			v = &read.PayloadView{Id: rec.Id}
			if rec.CreatedAt != nil {
				v.CreatedAt = rec.CreatedAt
			} else {
				t := time.Now()
				v.CreatedAt = &t
			}
		}
		if rec.Has != nil {
			if rec.Has.TenantID {
				v.TenantID = rec.TenantID
			}
			if rec.Has.Kind {
				v.Kind = rec.Kind
			}
			if rec.Has.Subtype {
				v.Subtype = rec.Subtype
			}
			if rec.Has.MimeType {
				v.MimeType = rec.MimeType
			}
			if rec.Has.SizeBytes {
				v.SizeBytes = rec.SizeBytes
			}
			if rec.Has.Digest {
				v.Digest = rec.Digest
			}
			if rec.Has.Storage {
				v.Storage = rec.Storage
			}
			if rec.Has.InlineBody {
				v.InlineBody = rec.InlineBody
			}
			if rec.Has.URI {
				v.URI = rec.URI
			}
			if rec.Has.Compression {
				v.Compression = rec.Compression
			}
			if rec.Has.Redacted {
				v.Redacted = rec.Redacted
			}
			if rec.Has.SchemaRef {
				v.SchemaRef = rec.SchemaRef
			}
			if rec.Has.Preview {
				v.Preview = rec.Preview
			}
			if rec.Has.Tags {
				v.Tags = rec.Tags
			}
		}
		// Enforce storage constraint similar to SQL handler defaults
		if v.Storage == "object" {
			v.InlineBody = nil
		}
		if v.Storage == "inline" {
			v.URI = nil
		}
		s.payload[rec.Id] = v
	}
	return &write.Output{Data: payloads}, nil
}

func match(v *read.PayloadView, in *read.Input) bool {
	if in.Has == nil {
		return true
	}
	if in.Has.TenantID && (v.TenantID == nil || *v.TenantID != in.TenantID) {
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
	if in.Has.Kind && v.Kind != in.Kind {
		return false
	}
	if in.Has.Digest && (v.Digest == nil || *v.Digest != in.Digest) {
		return false
	}
	if in.Has.Storage && v.Storage != in.Storage {
		return false
	}
	if in.Has.MimeType && v.MimeType != in.MimeType {
		return false
	}
	if in.Has.Since && in.Since != nil && v.CreatedAt != nil && v.CreatedAt.Before(*in.Since) {
		return false
	}
	return true
}

func clone(v *read.PayloadView) *read.PayloadView {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

var _ api.API = (*Service)(nil)

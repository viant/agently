package sql

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/viant/agently/internal/dao/payload"
	read2 "github.com/viant/agently/internal/dao/payload/read"
	write2 "github.com/viant/agently/internal/dao/payload/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

func Register(ctx context.Context, dao *datly.Service) error { return payload.Register(ctx, dao) }

func (s *Service) List(ctx context.Context, opts ...read2.InputOption) ([]*read2.PayloadView, error) {
	in := &read2.Input{}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	if in.Has != nil && in.Has.TenantID && in.TenantID != "" {
		uri := strings.ReplaceAll(read2.PathByTenant, "{tenantId}", in.TenantID)
		_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
		if err != nil {
			return nil, err
		}
		// auto-decompress
		for _, pv := range out.Data {
			if pv != nil && pv.Compression == "gzip" && pv.InlineBody != nil {
				gr, err := gzip.NewReader(bytes.NewReader(*pv.InlineBody))
				if err == nil {
					var buf bytes.Buffer
					_, _ = buf.ReadFrom(gr)
					_ = gr.Close()
					b := buf.Bytes()
					pv.InlineBody = &b
				}
			}
		}
		return out.Data, nil
	}
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(read2.PathBase), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	for _, pv := range out.Data {
		if pv != nil && pv.Compression == "gzip" && pv.InlineBody != nil {
			gr, err := gzip.NewReader(bytes.NewReader(*pv.InlineBody))
			if err == nil {
				var buf bytes.Buffer
				_, _ = buf.ReadFrom(gr)
				_ = gr.Close()
				b := buf.Bytes()
				pv.InlineBody = &b
			}
		}
	}
	return out.Data, nil
}

func (s *Service) Patch(ctx context.Context, payloads ...*write2.Payload) (*write2.Output, error) {
	in := &write2.Input{Payloads: payloads}
	out := &write2.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath(http.MethodPatch, write2.PathURI)),
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
type PayloadView = read2.PayloadView

func WithTenantID(id string) read2.InputOption { return read2.WithTenantID(id) }
func WithID(id string) read2.InputOption       { return read2.WithID(id) }
func WithIDs(ids ...string) read2.InputOption  { return read2.WithIDs(ids...) }
func WithKind(kind string) read2.InputOption   { return read2.WithKind(kind) }
func WithDigest(d string) read2.InputOption    { return read2.WithDigest(d) }
func WithStorage(s string) read2.InputOption   { return read2.WithStorage(s) }
func WithMimeType(m string) read2.InputOption  { return read2.WithMimeType(m) }
func WithSince(ts time.Time) read2.InputOption { return read2.WithSince(ts) }

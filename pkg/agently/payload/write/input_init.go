package write

import (
	"context"
	"time"

	"bytes"
	"compress/gzip"

	"github.com/viant/xdatly/handler"
)

func (i *Input) Init(ctx context.Context, sess handler.Session, _ *Output) error {
	if err := sess.Stater().Bind(ctx, i); err != nil {
		return err
	}
	i.indexSlice()
	for _, p := range i.Payloads {
		if p == nil {
			continue
		}
		if p.Has == nil {
			p.Has = &PayloadHas{}
		}
		if _, ok := i.CurByID[p.Id]; !ok {
			now := time.Now()
			p.CreatedAt = &now
			p.Has.CreatedAt = true
			if !p.Has.Compression {
				p.Compression = "none"
				p.Has.Compression = true
			}
			if p.Redacted == nil {
				zero := 0
				p.Redacted = &zero
				p.Has.Redacted = true
			}
		}
		// Compress large inline bodies (>1k) with gzip
		if p.Has != nil && p.Has.InlineBody && p.InlineBody != nil {
			if (!p.Has.Compression || p.Compression == "none") && (p.Has.Storage && p.Storage == "inline" || !p.Has.Storage) {
				if len(*p.InlineBody) > 1024 {
					var buf bytes.Buffer
					gw := gzip.NewWriter(&buf)
					_, _ = gw.Write(*p.InlineBody)
					_ = gw.Close()
					p.SetInlineBody(buf.Bytes())
					p.SetCompression("gzip")
					p.SetSizeBytes(len(buf.Bytes()))
				}
			}
		}
		if p.Has.Storage {
			switch p.Storage {
			case "object":
				// clear inline body when storing as object
				p.InlineBody = nil
				p.Has.InlineBody = true
			case "inline":
				// clear URI when storing inline
				p.URI = nil
				p.Has.URI = true
			}
		}
	}
	return nil
}

func (i *Input) indexSlice() {
	i.CurByID = map[string]*Payload{}
	for _, it := range i.Cur {
		if it != nil {
			i.CurByID[it.Id] = it
		}
	}
}

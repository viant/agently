package conversation

import (
	"bytes"
	"compress/gzip"
	"context"
)

func (p *PayloadView) OnFetch(ctx context.Context) error {
	if p.Compression == "gzip" && p.InlineBody != nil {
		gr, err := gzip.NewReader(bytes.NewReader([]byte(*p.InlineBody)))
		if err == nil {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(gr)
			_ = gr.Close()
			b := buf.Bytes()
			body := string(b)
			p.InlineBody = &body
			p.Compression = "none"
		}
	}
	return nil
}

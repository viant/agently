package read

import (
	"bytes"
	"compress/gzip"
	"context"
)

func (p *PayloadView) OnFetch(ctx context.Context) error {
	if p.InlineBody == nil {
		return nil
	}
	inline := []byte(*p.InlineBody)
	uncompressIfNeeded(&p.Compression, &inline)
	*p.InlineBody = bytes.TrimSpace(inline)
	return nil
}

func uncompressIfNeeded(compression *string, inlineBody *[]byte) {
	if *compression == "gzip" && inlineBody != nil {
		gr, err := gzip.NewReader(bytes.NewReader([]byte(*inlineBody)))
		if err == nil {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(gr)
			_ = gr.Close()
			b := buf.Bytes()
			*inlineBody = b
			*compression = ""
		}
	}
}
